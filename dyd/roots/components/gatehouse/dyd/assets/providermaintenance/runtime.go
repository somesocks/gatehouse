// Package providermaintenance runs agent-provider upkeep through DBOS. Scheduling
// lives in DBOS; provider rows hold only configuration and credential state.
package providermaintenance

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gatehouse/codexlogin"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

const (
	ScheduleName  = "gatehouse.agent-provider-maintenance"
	Schedule      = "0 0 * * * *"
	queueName     = "gatehouse.agent-provider-maintenance"
	refreshWindow = time.Hour + 5*time.Minute
)

type Refresher interface {
	Refresh(context.Context, codexlogin.Credential) (codexlogin.Credential, error)
}

type maintenanceStore interface {
	AgentProviderMaintenanceTargets(context.Context) (error, []database.AgentProviderMaintenanceTarget)
	AgentProviderMaintenanceGet(context.Context, string) (error, *database.AgentProviderMaintenanceState)
	AgentProviderCredentialGet(context.Context, string) (error, *database.AgentProviderCredential)
	AgentProviderMaintenanceCredentialSet(context.Context, string, string, model.KeychainRef, *string, *database.SystemAgentProviderAccount) (error, bool)
}

type Check struct {
	ProviderID  string
	Fingerprint string
	Force       bool
}

// Result is deliberately limited to non-secret metadata. Authorization bundles
// are never arguments, results, events, or DBOS checkpoint values.
type Result struct {
	ProviderID string
	State      string
	Reason     string
}

type handler func(context.Context, Check, *database.AgentProviderMaintenanceState) Result

type Runtime struct {
	dbos           dbos.Context
	store          maintenanceStore
	ring           *keychain.Keyring
	refresher      Refresher
	queue          dbos.Queue
	sweep          func(dbos.Context, dbos.ScheduledWorkflowInput) (any, error)
	maintain       func(dbos.Context, Check) (Result, error)
	handlers       map[string]handler
	now            func() time.Time
	persistTimeout time.Duration
}

func NewRuntime(ctx dbos.Context, store *database.Store, ring *keychain.Keyring, refresher Refresher) (error, *Runtime) {
	queue, err := dbos.RegisterQueue(ctx, queueName, dbos.WithPartitionConcurrency(1))
	if err != nil {
		return fmt.Errorf("register provider maintenance queue: %w", err), nil
	}
	if refresher == nil {
		refresher = codexlogin.NewClient(nil)
	}
	runtime := &Runtime{dbos: ctx, store: store, ring: ring, refresher: refresher, queue: queue, now: time.Now, persistTimeout: 10 * time.Second}
	runtime.handlers = map[string]handler{
		"builtin": func(_ context.Context, input Check, _ *database.AgentProviderMaintenanceState) Result {
			return result(input, "ready", "")
		},
		"openai-chat-completions":               runtime.checkAPIKey,
		"openai-responses":                      runtime.checkAPIKey,
		config.AgentProviderProtocolOpenAICodex: runtime.checkCodex,
	}
	runtime.sweep = func(ctx dbos.Context, input dbos.ScheduledWorkflowInput) (any, error) {
		return runtime.runSweep(ctx, input)
	}
	runtime.maintain = func(ctx dbos.Context, input Check) (Result, error) {
		return dbos.RunAsStep(ctx, func(step context.Context) (Result, error) {
			if err := step.Err(); err != nil {
				return Result{}, err
			}
			return runtime.check(step, input), nil
		}, dbos.WithStepName("gatehouse.provider-maintenance.check.v1"), dbos.WithStepMaxRetries(0))
	}
	dbos.RegisterWorkflow(ctx, runtime.sweep, dbos.WithWorkflowName("gatehouse.provider-maintenance.sweep.v1"))
	dbos.RegisterWorkflow(ctx, runtime.maintain, dbos.WithWorkflowName("gatehouse.provider-maintenance.check.v1"))
	return nil, runtime
}

// Call after Launch. A single application-wide schedule is shared by executors.
// Missed historical hours are not backfilled: checks always inspect live state.
func (runtime *Runtime) Start() error {
	return dbos.ApplySchedules(runtime.dbos, []dbos.ScheduleSpec{{ScheduleName: ScheduleName, Schedule: Schedule, CronTimezone: "UTC", Workflow: runtime.sweep}})
}

func (runtime *Runtime) runSweep(ctx dbos.Context, input dbos.ScheduledWorkflowInput) (any, error) {
	if input.ScheduledTime.IsZero() {
		return nil, errors.New("provider maintenance requires a scheduled tick")
	}
	targets, err := dbos.RunAsStep(ctx, func(step context.Context) ([]database.AgentProviderMaintenanceTarget, error) {
		err, targets := runtime.store.AgentProviderMaintenanceTargets(step)
		if err != nil {
			return nil, errors.New("provider maintenance inventory unavailable")
		}
		return targets, nil
	}, dbos.WithStepName("gatehouse.provider-maintenance.targets.v1"), dbos.WithStepMaxRetries(3))
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		id := "provider-maintenance:" + target.ID + ":" + input.ScheduledTime.UTC().Format(time.RFC3339Nano)
		if _, err := runtime.enqueue(ctx, id, Check{ProviderID: target.ID, Fingerprint: target.Fingerprint}); err != nil {
			return nil, err
		}
	}
	return len(targets), nil
}

func (runtime *Runtime) enqueue(ctx dbos.Context, id string, input Check) (dbos.WorkflowHandle[Result], error) {
	return dbos.RunWorkflow(ctx, runtime.maintain, input, dbos.WithWorkflowID(id), dbos.WithQueue(runtime.queue), dbos.WithQueuePartitionKey(input.ProviderID))
}

// CheckNow lets a future inference caller request early maintenance (e.g. on
// expiry/401) through the same queue. requestID must identify that request;
// retries with the same ID converge on the same durable job. Waiting/cancelling
// a caller does not cancel the shared job, which starts on the runtime context.
func (runtime *Runtime) CheckNow(providerID, requestID string) (dbos.WorkflowHandle[Result], error) {
	if strings.TrimSpace(requestID) == "" {
		return nil, errors.New("maintenance request ID is required")
	}
	err, state := runtime.store.AgentProviderMaintenanceGet(runtime.dbos, providerID)
	if err != nil {
		return nil, errors.New("provider maintenance inventory unavailable")
	}
	if state == nil {
		return nil, errors.New("agent provider is unavailable")
	}
	hash := sha256.Sum256([]byte(requestID))
	id := "provider-maintenance-now:" + providerID + ":" + hex.EncodeToString(hash[:])
	return runtime.enqueue(runtime.dbos, id, Check{ProviderID: providerID, Fingerprint: state.Fingerprint, Force: true})
}

func result(input Check, state, reason string) Result {
	return Result{ProviderID: input.ProviderID, State: state, Reason: reason}
}

func (runtime *Runtime) check(ctx context.Context, input Check) Result {
	err, state := runtime.store.AgentProviderMaintenanceGet(ctx, input.ProviderID)
	if err != nil {
		return result(input, "retry", "provider_read_failed")
	}
	if state == nil || !state.Provider.Enabled {
		return result(input, "skipped", "provider_unavailable")
	}
	if state.Fingerprint != input.Fingerprint {
		return result(input, "superseded", "")
	}
	handler, ok := runtime.handlers[state.Provider.Protocol]
	if !ok {
		return result(input, "unsupported", "unsupported_protocol")
	}
	return handler(ctx, input, state)
}

// Static-key protocols have no standard refresh or health-probe URL. Check local
// credential usability; do not infer /models or send paid inference requests.
func (runtime *Runtime) checkAPIKey(ctx context.Context, input Check, _ *database.AgentProviderMaintenanceState) Result {
	err, record := runtime.store.AgentProviderCredentialGet(ctx, input.ProviderID)
	if err != nil {
		return result(input, "retry", "provider_read_failed")
	}
	if record == nil || record.Fingerprint() != input.Fingerprint {
		return result(input, "superseded", "")
	}
	if !record.Provider.Enabled {
		return result(input, "skipped", "provider_unavailable")
	}
	if record.Encrypted == nil {
		return result(input, "disconnected", "credentials_required")
	}
	plain, err := runtime.open(ctx, record)
	if err != nil {
		return result(input, "unavailable", "credential_unreadable")
	}
	defer clear(plain)
	if strings.TrimSpace(string(plain)) == "" {
		return result(input, "unavailable", "credential_unreadable")
	}
	return result(input, "credentials_available", "")
}

func (runtime *Runtime) checkCodex(ctx context.Context, input Check, state *database.AgentProviderMaintenanceState) Result {
	if !state.Provider.CredentialConfigured {
		return result(input, "disconnected", "credentials_required")
	}
	if !input.Force && state.Provider.Account != nil && !state.Provider.Account.ExpiresAt.IsZero() && state.Provider.Account.ExpiresAt.After(runtime.now().Add(refreshWindow)) {
		return result(input, "unchanged", "")
	}
	err, record := runtime.store.AgentProviderCredentialGet(ctx, input.ProviderID)
	if err != nil {
		return result(input, "retry", "provider_read_failed")
	}
	if record == nil || record.Fingerprint() != input.Fingerprint {
		return result(input, "superseded", "")
	}
	if !record.Provider.Enabled {
		return result(input, "skipped", "provider_unavailable")
	}
	if record.Encrypted == nil {
		return result(input, "disconnected", "credentials_required")
	}
	plain, err := runtime.open(ctx, record)
	if err != nil {
		return result(input, "unavailable", "credential_unreadable")
	}
	defer clear(plain)
	var previous codexlogin.Credential
	if json.Unmarshal(plain, &previous) != nil || previous.Validate() != nil {
		return result(input, "unavailable", "credential_invalid")
	}
	if !input.Force && !previous.ExpiresAt.IsZero() && previous.ExpiresAt.After(runtime.now().Add(refreshWindow)) {
		return result(input, "unchanged", "")
	}
	next, err := runtime.refresher.Refresh(ctx, previous)
	if err != nil {
		var failure *codexlogin.RefreshError
		if errors.As(err, &failure) && failure.Permanent {
			return runtime.persist(ctx, input, record, nil, nil, "reconnect_required")
		}
		return result(input, "retry", "refresh_unavailable")
	}
	if next.Validate() != nil || next.AccountID != previous.AccountID || (!next.ExpiresAt.IsZero() && !next.ExpiresAt.After(runtime.now())) {
		return result(input, "retry", "refresh_response_invalid")
	}
	payload, err := json.Marshal(next)
	if err != nil {
		return result(input, "retry", "refresh_response_invalid")
	}
	defer clear(payload)
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), runtime.persistTimeout)
	defer cancel()
	encrypted, err := runtime.seal(cleanup, record, payload)
	if err != nil {
		return result(input, "retry", "credential_unreadable")
	}
	account := &database.SystemAgentProviderAccount{ID: next.AccountID, Email: next.Email, ExpiresAt: next.ExpiresAt}
	return runtime.persist(ctx, input, record, &encrypted, account, "refreshed")
}

// Retry persistence of the SAME exchanged bundle, not another token POST. Finish
// a successful exchange under a bounded cleanup context even if the caller ends.
func (runtime *Runtime) persist(ctx context.Context, input Check, record *database.AgentProviderCredential, encrypted *string, account *database.SystemAgentProviderAccount, state string) Result {
	if record.Provider.Keychain == nil || record.Encrypted == nil {
		return result(input, "unavailable", "credential_unreadable")
	}
	write, cancel := context.WithTimeout(context.WithoutCancel(ctx), runtime.persistTimeout)
	defer cancel()
	for {
		err, changed := runtime.store.AgentProviderMaintenanceCredentialSet(write, input.ProviderID, *record.Encrypted, *record.Provider.Keychain, encrypted, account)
		if err == nil {
			if !changed {
				return result(input, "superseded", "")
			}
			return result(input, state, "")
		}
		select {
		case <-write.Done():
			return result(input, "retry", "credential_save_failed")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func associatedData(record *database.AgentProviderCredential, selector string) ([]byte, error) {
	switch selector {
	case keychain.AADID:
		return []byte("gh=v1|agent-provider|id=" + record.Provider.ID), nil
	case keychain.AADAlias:
		if record.Provider.Alias != "" {
			return []byte("gh=v1|agent-provider|alias=" + record.Provider.Alias), nil
		}
	}
	return nil, errors.New("provider credential associated data is unavailable")
}

func (runtime *Runtime) open(ctx context.Context, record *database.AgentProviderCredential) ([]byte, error) {
	if runtime.ring == nil || record.Provider.Keychain == nil || record.Encrypted == nil {
		return nil, errors.New("provider keychain unavailable")
	}
	err, encrypted := keychain.ParseKey(*record.Encrypted)
	if err != nil {
		return nil, errors.New("provider credential unreadable")
	}
	aad, err := associatedData(record, encrypted.AAD)
	if err != nil {
		return nil, err
	}
	err, keys := runtime.ring.Get(ctx, []model.KeychainRef{*record.Provider.Keychain})
	if err != nil {
		return nil, errors.New("provider keychain unavailable")
	}
	defer clear(keys[*record.Provider.Keychain])
	err, plain := keychain.Open(keys[*record.Provider.Keychain], aad, encrypted)
	if err != nil {
		return nil, errors.New("provider credential unreadable")
	}
	return plain, nil
}

func (runtime *Runtime) seal(ctx context.Context, record *database.AgentProviderCredential, plain []byte) (string, error) {
	if runtime.ring == nil || record.Provider.Keychain == nil || record.Encrypted == nil {
		return "", errors.New("provider keychain unavailable")
	}
	err, original := keychain.ParseKey(*record.Encrypted)
	if err != nil {
		return "", errors.New("provider credential unreadable")
	}
	aad, err := associatedData(record, original.AAD)
	if err != nil {
		return "", err
	}
	err, keys := runtime.ring.Get(ctx, []model.KeychainRef{*record.Provider.Keychain})
	if err != nil {
		return "", errors.New("provider keychain unavailable")
	}
	defer clear(keys[*record.Provider.Keychain])
	err, encrypted := keychain.Seal(rand.Reader, keys[*record.Provider.Keychain], aad, plain)
	if err != nil {
		return "", errors.New("provider credential could not be encrypted")
	}
	encrypted.AAD = original.AAD
	return encrypted.String(), nil
}
