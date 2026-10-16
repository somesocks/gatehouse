package providermaintenance

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gatehouse/codexlogin"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	_ "github.com/dbos-inc/dbos-transact-golang/dbos/driver/sqlite"
)

type refreshFunc func(context.Context, codexlogin.Credential) (codexlogin.Credential, error)

func (fn refreshFunc) Refresh(ctx context.Context, credential codexlogin.Credential) (codexlogin.Credential, error) {
	return fn(ctx, credential)
}

type fixture struct {
	configuration config.DatabaseConfig
	store         *database.Store
	ring          *keychain.Keyring
	runtime       *Runtime
	ctx           dbos.Context
}

func newFixture(t *testing.T, refresher Refresher) *fixture {
	return newConfiguredFixture(t, config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "maintenance.sqlite")}, refresher)
}

func newConfiguredFixture(t *testing.T, configuration config.DatabaseConfig, refresher Refresher) *fixture {
	t.Helper()
	t.Setenv("DBOS__APPVERSION", "")
	t.Setenv("DBOS__VMID", "")
	t.Setenv("MAINTENANCE_TEST_KEYCHAIN", "test passphrase")
	err, store := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	configured := []config.Keychain{{ID: "maintenance", Sources: []config.KeychainPassphraseSource{"env:MAINTENANCE_TEST_KEYCHAIN"}}}
	err, ring := keychain.NewKeyring(store, configured, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ring.Close)
	err, registry := migrations.Build(configuration, config.State{Keychains: configured}, ring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(context.Background(), store, registry); err != nil {
		t.Fatal(err)
	}
	ctx, runtime := launch(t, store, ring, refresher, "first")
	return &fixture{configuration: configuration, store: store, ring: ring, runtime: runtime, ctx: ctx}
}

func launch(t *testing.T, store *database.Store, ring *keychain.Keyring, refresher Refresher, executor string) (dbos.Context, *Runtime) {
	t.Helper()
	configuration := dbos.Config{AppName: "provider-maintenance-tests", ApplicationVersion: "maintenance-v1", ExecutorID: executor, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if store.PostgresPool() != nil {
		configuration.DatabaseURL = store.DatabaseURL()
	} else {
		configuration.SQLiteSystemDB = store.DB
	}
	ctx, err := dbos.NewContext(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	err, runtime := NewRuntime(ctx, store, ring, refresher)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.queue.SetPollingInterval(ctx, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := dbos.Launch(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := dbos.Shutdown(ctx, 2*time.Second); err != nil {
			t.Errorf("shutdown maintenance runtime: %v", err)
		}
	})
	return ctx, runtime
}

func (f *fixture) add(t *testing.T, alias string, expires time.Time) *database.SystemAgentProvider {
	t.Helper()
	err, provider := f.store.SystemAgentProviderCreate(context.Background(), database.SystemAgentProviderInput{Alias: alias, Protocol: config.AgentProviderProtocolOpenAICodex, Keychain: &model.KeychainRef{Id: "maintenance", Version: 1}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return f.install(t, provider, codexlogin.Credential{Version: 1, AccessToken: "ACCESS_" + alias + "_SENTINEL", RefreshToken: "REFRESH_" + alias + "_SENTINEL", AccountID: alias, Email: alias + "@example.com", ExpiresAt: expires})
}

func (f *fixture) install(t *testing.T, provider *database.SystemAgentProvider, credential codexlogin.Credential) *database.SystemAgentProvider {
	t.Helper()
	body, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	text := f.seal(t, provider, body)
	clear(body)
	err, updated := f.store.SystemAgentProviderCodexCredentialSet(context.Background(), provider.ID, provider.Revision, provider.Keychain, &text, &database.SystemAgentProviderAccount{ID: credential.AccountID, Email: credential.Email, ExpiresAt: credential.ExpiresAt})
	if err != nil {
		t.Fatal(err)
	}
	return updated
}

func (f *fixture) seal(t *testing.T, provider *database.SystemAgentProvider, body []byte) string {
	t.Helper()
	err, keys := f.ring.Get(context.Background(), []model.KeychainRef{*provider.Keychain})
	if err != nil {
		t.Fatal(err)
	}
	defer clear(keys[*provider.Keychain])
	err, encrypted := keychain.Seal(rand.Reader, keys[*provider.Keychain], []byte("gh=v1|agent-provider|alias="+provider.Alias), body)
	if err != nil {
		t.Fatal(err)
	}
	encrypted.AAD = keychain.AADAlias
	return encrypted.String()
}

func (f *fixture) record(t *testing.T, id string) *database.AgentProviderCredential {
	t.Helper()
	err, record := f.store.AgentProviderCredentialGet(context.Background(), id)
	if err != nil || record == nil {
		t.Fatalf("read credential: %v", err)
	}
	return record
}

func (f *fixture) credential(t *testing.T, id string) codexlogin.Credential {
	t.Helper()
	plain, err := f.runtime.open(context.Background(), f.record(t, id))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(plain)
	var credential codexlogin.Credential
	if err := json.Unmarshal(plain, &credential); err != nil {
		t.Fatal(err)
	}
	return credential
}

func await(t *testing.T, handle dbos.WorkflowHandle[Result]) Result {
	t.Helper()
	result, err := handle.GetResult(dbos.WithHandleTimeout(15*time.Second), dbos.WithHandlePollingInterval(10*time.Millisecond))
	if err != nil {
		status, _ := handle.GetStatus()
		t.Fatalf("await maintenance (%s): %v", status.Status, err)
	}
	return result
}

func (f *fixture) check(t *testing.T, provider *database.SystemAgentProvider, request string) Result {
	t.Helper()
	handle, err := f.runtime.CheckNow(provider.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	return await(t, handle)
}

func (f *fixture) sweep(t *testing.T, tick time.Time) map[string]Result {
	t.Helper()
	handle, err := dbos.RunWorkflow(f.ctx, f.runtime.sweep, dbos.ScheduledWorkflowInput{ScheduledTime: tick}, dbos.WithWorkflowID("test-sweep:"+tick.Format(time.RFC3339Nano)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.GetResult(dbos.WithHandleTimeout(15 * time.Second)); err != nil {
		t.Fatal(err)
	}
	err, targets := f.store.AgentProviderMaintenanceTargets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Result{}
	for _, target := range targets {
		handle, err := dbos.RetrieveWorkflow[Result](f.ctx, "provider-maintenance:"+target.ID+":"+tick.UTC().Format(time.RFC3339Nano))
		if err != nil {
			t.Fatal(err)
		}
		out[target.ID] = await(t, handle)
	}
	return out
}

func TestHourlySweepRefreshesOnlyDueCredentials(t *testing.T) {
	var calls atomic.Int32
	f := newFixture(t, refreshFunc(func(_ context.Context, old codexlogin.Credential) (codexlogin.Credential, error) {
		calls.Add(1)
		old.AccessToken = "NEW_ACCESS_SENTINEL"
		old.RefreshToken = "ROTATED_REFRESH_SENTINEL"
		return old, nil
	}))
	unknown := f.add(t, "unknown", time.Time{})
	initialFingerprint := f.record(t, unknown.ID).Fingerprint()
	fresh := f.add(t, "fresh", time.Now().Add(8*24*time.Hour))
	disabled := f.add(t, "disabled", time.Time{})
	err, _ := f.store.SystemAgentProviderUpdate(context.Background(), disabled.ID, disabled.Revision, database.SystemAgentProviderInput{Alias: disabled.Alias, Protocol: disabled.Protocol, Keychain: disabled.Keychain, Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	err, builtin := f.store.SystemAgentProviderCreate(context.Background(), database.SystemAgentProviderInput{Alias: "builtin", Protocol: "builtin", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	tick := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	results := f.sweep(t, tick)
	if results[unknown.ID].State != "refreshed" || results[fresh.ID].State != "unchanged" || results[builtin.ID].State != "ready" || len(results) != 3 || calls.Load() != 1 {
		t.Fatalf("sweep results = %#v, calls=%d", results, calls.Load())
	}
	f.sweep(t, tick) // Same tick reuses completed DBOS work, including unknown expiry.
	if calls.Load() != 1 {
		t.Fatal("duplicate tick refreshed twice")
	}
	f.sweep(t, tick.Add(time.Hour))
	if calls.Load() != 2 {
		t.Fatal("unknown expiry was not refreshed next hour")
	}
	record := f.record(t, unknown.ID)
	if record.Provider.Revision != unknown.Revision || record.Fingerprint() == initialFingerprint || !record.Provider.Account.ExpiresAt.IsZero() {
		t.Fatal("maintenance changed revision or fabricated expiry")
	}
	if current := f.credential(t, unknown.ID); current.RefreshToken != "ROTATED_REFRESH_SENTINEL" || !current.ExpiresAt.IsZero() {
		t.Fatal("rotated bundle not saved")
	}
	assertNoSecrets(t, f.store, "ACCESS_unknown_SENTINEL", "REFRESH_unknown_SENTINEL", "NEW_ACCESS_SENTINEL", "ROTATED_REFRESH_SENTINEL", *record.Encrypted)
}

func TestScheduleIsOneHourlyDBOSCron(t *testing.T) {
	f := newFixture(t, nil)
	if err := f.runtime.Start(); err != nil {
		t.Fatal(err)
	}
	if err := f.runtime.Start(); err != nil {
		t.Fatal(err)
	}
	schedules, err := dbos.ListSchedules(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(schedules) != 1 || schedules[0].Schedule != Schedule || schedules[0].CronTimezone != "UTC" || schedules[0].ScheduleName != ScheduleName || schedules[0].AutomaticBackfill {
		t.Fatalf("schedules = %#v", schedules)
	}
	if err, _ := f.store.SystemAgentProviderCreate(context.Background(), database.SystemAgentProviderInput{Alias: "scheduled", Protocol: "builtin", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// Enqueue one native scheduled tick without waiting for the hourly clock.
	tick := time.Now().UTC().Truncate(time.Hour)
	ids, err := dbos.BackfillSchedule(f.ctx, ScheduleName, tick.Add(-time.Second), tick.Add(time.Second))
	if err != nil || len(ids) != 1 {
		t.Fatalf("backfill one scheduled tick: %v, %v", ids, err)
	}
	triggered, err := dbos.RetrieveWorkflow[any](f.ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	count, err := triggered.GetResult(dbos.WithHandleTimeout(15 * time.Second))
	if err != nil || fmt.Sprint(count) != "1" {
		t.Fatalf("native scheduled dispatch = %v, %v", count, err)
	}
	if limit := f.runtime.queue.GetPartitionConcurrency(); limit == nil || *limit != 1 {
		t.Fatal("queue lacks per-provider concurrency")
	}
	rows, err := f.store.Query(`PRAGMA table_info(gatehouse_agent_providers)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	allowed := map[string]bool{"id": true, "alias": true, "revision": true, "protocol": true, "endpoint": true, "keychain_id": true, "keychain_version": true, "credential": true, "credential_metadata": true, "enabled": true}
	for rows.Next() {
		var index, notnull, pk int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&index, &name, &kind, &notnull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		if !allowed[name] {
			t.Fatalf("provider schema changed for maintenance: %s", name)
		}
	}
}

func TestKnownExpiryWindowAndStaticCredentialChecks(t *testing.T) {
	var calls atomic.Int32
	f := newFixture(t, refreshFunc(func(_ context.Context, old codexlogin.Credential) (codexlogin.Credential, error) {
		calls.Add(1)
		old.ExpiresAt = time.Now().Add(24 * time.Hour)
		return old, nil
	}))
	due := f.add(t, "due", time.Now().Add(30*time.Minute))
	future := f.add(t, "future", time.Now().Add(3*time.Hour))
	for _, protocol := range []string{"openai-responses", "openai-chat-completions"} {
		alias := strings.TrimPrefix(protocol, "openai-")
		endpoint := "https://api.example.test/custom-route"
		provider := &database.SystemAgentProvider{Alias: alias, Keychain: &model.KeychainRef{Id: "maintenance", Version: 1}}
		key := f.seal(t, provider, []byte("STATIC_API_KEY_SENTINEL"))
		err, _ := f.store.SystemAgentProviderCreate(context.Background(), database.SystemAgentProviderInput{Alias: alias, Protocol: protocol, Endpoint: &endpoint, Keychain: provider.Keychain, EncryptedCredential: &key, Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
	}
	results := f.sweep(t, time.Now().UTC().Truncate(time.Hour))
	if results[due.ID].State != "refreshed" || results[future.ID].State != "unchanged" || calls.Load() != 1 {
		t.Fatal("expiry window was not respected")
	}
	static := 0
	for _, result := range results {
		if result.State == "credentials_available" {
			static++
		}
	}
	if static != 2 {
		t.Fatal("static protocol credential checks missing")
	}
}

func TestTransientFailureWaitsForNextSweepAndPermanentFailureRequiresReconnect(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		t.Run(fmt.Sprint(permanent), func(t *testing.T) {
			var calls atomic.Int32
			f := newFixture(t, refreshFunc(func(context.Context, codexlogin.Credential) (codexlogin.Credential, error) {
				calls.Add(1)
				return codexlogin.Credential{}, &codexlogin.RefreshError{Permanent: permanent}
			}))
			provider := f.add(t, "failure", time.Time{})
			before := f.record(t, provider.ID)
			tick := time.Now().UTC().Truncate(time.Hour)
			out := f.sweep(t, tick)[provider.ID]
			after := f.record(t, provider.ID)
			if after.Provider.Revision != provider.Revision {
				t.Fatal("refresh changed configuration revision")
			}
			if permanent {
				if out.State != "reconnect_required" || after.Encrypted != nil || after.Provider.Account != nil || after.Fingerprint() == before.Fingerprint() {
					t.Fatal("permanent rejection did not invalidate matching credential")
				}
			} else if out.State != "retry" || *after.Encrypted != *before.Encrypted || after.Fingerprint() != before.Fingerprint() {
				t.Fatal("transient failure changed credential")
			}
			f.sweep(t, tick)
			if calls.Load() != 1 {
				t.Fatal("same tick retried token POST")
			}
			f.sweep(t, tick.Add(time.Hour))
			want := int32(2)
			if permanent {
				want = 1
			}
			if calls.Load() != want {
				t.Fatal("incorrect next-sweep retry behavior")
			}
			assertNoSecrets(t, f.store, "ACCESS_failure_SENTINEL", "REFRESH_failure_SENTINEL", *before.Encrypted)
		})
	}
}

func TestReconnectDisconnectAndConfigurationChangesFenceRefresh(t *testing.T) {
	for _, change := range []string{"reconnect", "stale_rejection", "disconnect", "disable", "reconcile", "protocol"} {
		t.Run(change, func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			f := newFixture(t, refreshFunc(func(ctx context.Context, old codexlogin.Credential) (codexlogin.Credential, error) {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
					return codexlogin.Credential{}, ctx.Err()
				}
				if change == "stale_rejection" {
					return codexlogin.Credential{}, &codexlogin.RefreshError{Permanent: true}
				}
				old.AccessToken = "REFRESHED_ACCESS_SENTINEL"
				old.ExpiresAt = time.Now().Add(24 * time.Hour)
				return old, nil
			}))
			provider := f.add(t, "race", time.Time{})
			handle, err := f.runtime.CheckNow(provider.ID, "race")
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(10 * time.Second):
				t.Fatal("refresh did not start")
			}
			switch change {
			case "reconnect", "stale_rejection":
				f.install(t, provider, codexlogin.Credential{Version: 1, AccessToken: "RECONNECTED_ACCESS_SENTINEL", RefreshToken: "RECONNECTED_REFRESH_SENTINEL", AccountID: "replacement"})
			case "disconnect":
				if err, _ := f.store.SystemAgentProviderCodexCredentialSet(context.Background(), provider.ID, provider.Revision, nil, nil, nil); err != nil {
					t.Fatal(err)
				}
			case "disable":
				if err, _ := f.store.SystemAgentProviderUpdate(context.Background(), provider.ID, provider.Revision, database.SystemAgentProviderInput{Alias: provider.Alias, Protocol: provider.Protocol, Keychain: provider.Keychain, Enabled: false}); err != nil {
					t.Fatal(err)
				}
			case "protocol":
				if err, _ := f.store.SystemAgentProviderUpdate(context.Background(), provider.ID, provider.Revision, database.SystemAgentProviderInput{Alias: provider.Alias, Protocol: "builtin", Enabled: true}); err != nil {
					t.Fatal(err)
				}
			case "reconcile":
				key, endpoint := "maintenance", config.OpenAICodexEndpoint
				err, registry := migrations.Build(f.configuration, config.State{AgentProviders: []config.AgentProvider{{Alias: provider.Alias, Protocol: provider.Protocol, Revision: 10, Keychain: &key, Endpoint: &endpoint, Enabled: true}}}, f.ring)
				if err != nil {
					t.Fatal(err)
				}
				if err := migrations.Run(context.Background(), f.store, registry); err != nil {
					t.Fatal(err)
				}
			}
			close(release)
			out := await(t, handle)
			record := f.record(t, provider.ID)
			switch change {
			case "reconnect", "stale_rejection":
				if out.State != "superseded" || f.credential(t, provider.ID).AccessToken != "RECONNECTED_ACCESS_SENTINEL" {
					t.Fatal("stale refresh affected reconnect")
				}
			case "disconnect", "protocol":
				if out.State != "superseded" || record.Encrypted != nil {
					t.Fatal("stale refresh restored cleared credentials")
				}
			case "disable":
				if out.State != "refreshed" || record.Provider.Enabled || record.Provider.Revision != 3 {
					t.Fatal("refresh overwrote enabled/configuration state")
				}
			case "reconcile":
				if out.State != "refreshed" || record.Provider.Revision != 10 || f.credential(t, provider.ID).AccessToken != "REFRESHED_ACCESS_SENTINEL" {
					t.Fatal("reconciliation lost active refresh")
				}
			}
		})
	}
}

func TestMultipleExecutorsSerializeByProviderAndDeduplicateRequests(t *testing.T) {
	verifyExecutors(t, config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "executors.sqlite")})
}

func TestPostgresProviderMaintenance(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_MAINTENANCE_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	verifyExecutors(t, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_MAINTENANCE_POSTGRES_URL"})
}

func verifyExecutors(t *testing.T, configuration config.DatabaseConfig) {
	t.Helper()
	started, release := make(chan struct{}, 4), make(chan struct{})
	var callsA, activeA, maximumA atomic.Int32
	refresher := refreshFunc(func(ctx context.Context, old codexlogin.Credential) (codexlogin.Credential, error) {
		if old.AccountID == "a" {
			callsA.Add(1)
			active := activeA.Add(1)
			defer activeA.Add(-1)
			for current := maximumA.Load(); active > current && !maximumA.CompareAndSwap(current, active); current = maximumA.Load() {
			}
			started <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return codexlogin.Credential{}, ctx.Err()
			}
		}
		old.AccessToken = "MULTI_EXECUTOR_ACCESS_SENTINEL"
		return old, nil
	})
	f := newConfiguredFixture(t, configuration, refresher)
	a, b := f.add(t, "a", time.Time{}), f.add(t, "b", time.Time{})
	err, second := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { second.Close() })
	_, other := launch(t, second, f.ring, refresher, "second")
	first, err := f.runtime.CheckNow(a.ID, "first")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("first provider did not start")
	}
	queued, err := other.CheckNow(a.ID, "second")
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := other.CheckNow(a.ID, "first")
	if err != nil || duplicate.GetWorkflowID() != first.GetWorkflowID() {
		t.Fatal("request was not deduplicated")
	}
	parallel, err := other.CheckNow(b.ID, "parallel")
	if err != nil {
		t.Fatal(err)
	}
	if out := await(t, parallel); out.State != "refreshed" {
		t.Fatal("another provider could not run independently")
	}
	if callsA.Load() != 1 {
		t.Fatal("same provider ran concurrently")
	}
	close(release)
	if await(t, first).State != "refreshed" || await(t, duplicate).State != "refreshed" || await(t, queued).State != "superseded" || callsA.Load() != 1 || maximumA.Load() != 1 {
		t.Fatal("provider execution was not serialized")
	}
}

type flakyStore struct {
	*database.Store
	calls      int
	ciphertext string
	mu         sync.Mutex
}

func (store *flakyStore) AgentProviderMaintenanceCredentialSet(ctx context.Context, id string, original string, ref model.KeychainRef, encrypted *string, account *database.SystemAgentProviderAccount) (error, bool) {
	store.mu.Lock()
	store.calls++
	attempt := store.calls
	if encrypted != nil {
		if store.ciphertext != "" && store.ciphertext != *encrypted {
			store.mu.Unlock()
			return errors.New("bundle changed on retry"), false
		}
		store.ciphertext = *encrypted
	}
	store.mu.Unlock()
	if attempt == 1 {
		return errors.New("temporary database failure"), false
	}
	return store.Store.AgentProviderMaintenanceCredentialSet(ctx, id, original, ref, encrypted, account)
}

func TestSuccessfulExchangeRetriesSavingTheSameBundle(t *testing.T) {
	var calls atomic.Int32
	f := newFixture(t, refreshFunc(func(_ context.Context, old codexlogin.Credential) (codexlogin.Credential, error) {
		calls.Add(1)
		old.RefreshToken = "SAVE_RETRY_REFRESH_SENTINEL"
		return old, nil
	}))
	provider := f.add(t, "retry", time.Time{})
	store := &flakyStore{Store: f.store}
	f.runtime.store = store
	if out := f.check(t, provider, "retry"); out.State != "refreshed" || calls.Load() != 1 {
		t.Fatal("exchange repeated or save failed")
	}
	store.mu.Lock()
	saved := store.calls
	store.mu.Unlock()
	if saved != 2 || f.credential(t, provider.ID).RefreshToken != "SAVE_RETRY_REFRESH_SENTINEL" {
		t.Fatal("rotated bundle was not retried and retained")
	}
}

func TestFreshCiphertextFencesStaleWritesEvenForIdenticalPlaintext(t *testing.T) {
	f := newFixture(t, refreshFunc(func(_ context.Context, old codexlogin.Credential) (codexlogin.Credential, error) { return old, nil }))
	provider := f.add(t, "same", time.Time{})
	before := f.record(t, provider.ID)
	plaintext := f.credential(t, provider.ID)
	if out := f.check(t, provider, "same"); out.State != "refreshed" {
		t.Fatal("refresh did not save")
	}
	after := f.record(t, provider.ID)
	if f.credential(t, provider.ID) != plaintext || *after.Encrypted == *before.Encrypted || after.Fingerprint() == before.Fingerprint() {
		t.Fatal("encryption nonce did not distinguish replacement credentials")
	}
	err, changed := f.store.AgentProviderMaintenanceCredentialSet(context.Background(), provider.ID, *before.Encrypted, *provider.Keychain, nil, nil)
	if err != nil || changed {
		t.Fatal("stale rejection cleared a re-encrypted credential")
	}
	if current := f.record(t, provider.ID); current.Encrypted == nil || *current.Encrypted != *after.Encrypted {
		t.Fatal("stale write affected current ciphertext")
	}
}

type interruptStore struct {
	*database.Store
	saved chan struct{}
	step  context.Context
}

func (store *interruptStore) AgentProviderMaintenanceCredentialSet(ctx context.Context, id string, original string, ref model.KeychainRef, encrypted *string, account *database.SystemAgentProviderAccount) (error, bool) {
	err, changed := store.Store.AgentProviderMaintenanceCredentialSet(ctx, id, original, ref, encrypted, account)
	if err == nil && changed {
		close(store.saved)
		<-store.step.Done()
	}
	return err, changed
}

func TestRecoveryAfterSaveBeforeCheckpointDoesNotRefreshUnknownExpiryTwice(t *testing.T) {
	var calls atomic.Int32
	var interrupted *interruptStore
	f := newFixture(t, refreshFunc(func(ctx context.Context, old codexlogin.Credential) (codexlogin.Credential, error) {
		calls.Add(1)
		interrupted.step = ctx
		old.RefreshToken = "RECOVERED_REFRESH_SENTINEL"
		return old, nil
	}))
	provider := f.add(t, "recovery", time.Time{})
	interrupted = &interruptStore{Store: f.store, saved: make(chan struct{})}
	f.runtime.store = interrupted
	handle, err := f.runtime.CheckNow(provider.ID, "recovery")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-interrupted.saved:
	case <-time.After(10 * time.Second):
		t.Fatal("credential was not saved")
	}
	if err := dbos.Shutdown(f.ctx, 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	err, reopened := database.Open(context.Background(), f.configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	err, ring := keychain.NewKeyring(reopened, []config.Keychain{{ID: "maintenance", Sources: []config.KeychainPassphraseSource{"env:MAINTENANCE_TEST_KEYCHAIN"}}}, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ring.Close)
	ctx, recoveredRuntime := launch(t, reopened, ring, refreshFunc(func(context.Context, codexlogin.Credential) (codexlogin.Credential, error) {
		calls.Add(1)
		return codexlogin.Credential{}, errors.New("unexpected repeated exchange")
	}), "first")
	f.store, f.ring, f.runtime = reopened, ring, recoveredRuntime
	recovered, err := dbos.RetrieveWorkflow[Result](ctx, handle.GetWorkflowID())
	if err != nil {
		t.Fatal(err)
	}
	if out := await(t, recovered); out.State != "superseded" || calls.Load() != 1 || f.credential(t, provider.ID).RefreshToken != "RECOVERED_REFRESH_SENTINEL" {
		t.Fatal("recovery repeated token exchange")
	}
	assertNoSecrets(t, reopened, "REFRESH_recovery_SENTINEL", "RECOVERED_REFRESH_SENTINEL")
}

func assertNoSecrets(t *testing.T, store *database.Store, secrets ...string) {
	t.Helper()
	rows, err := store.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'gatehouse_%' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		t.Fatal(err)
	}
	tables := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	rows.Close()
	for _, table := range tables {
		rows, err := store.Query(`SELECT * FROM "` + strings.ReplaceAll(table, `"`, `""`) + `"`)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(values))
			for index := range values {
				pointers[index] = &values[index]
			}
			if err := rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			for _, value := range values {
				var text string
				switch value := value.(type) {
				case string:
					text = value
				case []byte:
					text = string(value)
				}
				decoded, _ := base64.StdEncoding.DecodeString(text)
				for _, secret := range secrets {
					if secret != "" && (strings.Contains(text, secret) || strings.Contains(string(decoded), secret)) {
						t.Fatalf("credential material leaked into DBOS table %s", table)
					}
				}
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
}
