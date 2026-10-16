package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gatehouse/config"
	"gatehouse/model"
	"gatehouse/typed_id"
)

var (
	ErrRevisionConflict       = errors.New("revision conflict")
	ErrAgentProviderExists    = errors.New("agent provider already exists")
	ErrAgentModelExists       = errors.New("agent model already exists")
	ErrWorkspaceAgentExists   = errors.New("workspace agent already exists")
	ErrAdminDependencyMissing = errors.New("administration dependency not found")
)

var administrationAlias = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
var workspaceAgentAlias = regexp.MustCompile(`^[a-z0-9_-]+(/[a-z0-9_-]+)*$`)

type SystemAgentProvider struct {
	ID                   string                      `json:"id"`
	Alias                string                      `json:"alias"`
	Revision             int                         `json:"revision"`
	Protocol             string                      `json:"protocol"`
	Endpoint             *string                     `json:"endpoint,omitempty"`
	Keychain             *model.KeychainRef          `json:"keychain,omitempty"`
	CredentialConfigured bool                        `json:"credential_configured"`
	Enabled              bool                        `json:"enabled"`
	Account              *SystemAgentProviderAccount `json:"account,omitempty"`
}

type SystemAgentProviderAccount struct {
	ID        string    `json:"id"`
	Email     string    `json:"email,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitzero"`
}

type SystemAgentProviderInput struct {
	Alias               string
	Protocol            string
	Endpoint            *string
	Keychain            *model.KeychainRef
	EncryptedCredential *string
	Enabled             bool
}

type SystemAgentModel struct {
	ID                string  `json:"id"`
	Alias             string  `json:"alias"`
	Revision          int     `json:"revision"`
	ProviderID        string  `json:"provider"`
	ProviderAlias     *string `json:"provider_alias,omitempty"`
	Model             string  `json:"model"`
	Parameters        string  `json:"parameters"`
	Compaction        string  `json:"compaction"`
	MaxTurns          int     `json:"max_turns"`
	MaxOutputTokens   int     `json:"max_output_tokens"`
	ResultInlineBytes int     `json:"result_inline_bytes"`
	Enabled           bool    `json:"enabled"`
}

type SystemAgentModelInput struct {
	Alias             string
	ProviderID        string
	Model             string
	Parameters        string
	Compaction        string
	MaxTurns          int
	MaxOutputTokens   int
	ResultInlineBytes *int
	Enabled           bool
}

type SystemWorkspaceAgent struct {
	ID           string                           `json:"id"`
	WorkspaceID  string                           `json:"workspace"`
	Alias        string                           `json:"alias"`
	ModelID      string                           `json:"model"`
	ModelAlias   *string                          `json:"model_alias,omitempty"`
	Revision     int                              `json:"revision"`
	Label        *string                          `json:"label,omitempty"`
	SystemPrompt *string                          `json:"system_prompt,omitempty"`
	Prelude      *string                          `json:"prelude,omitempty"`
	RateLimits   *config.WorkspaceAgentRateLimits `json:"rate_limits,omitempty"`
	Default      bool                             `json:"default"`
	Enabled      bool                             `json:"enabled"`
}

type SystemWorkspaceAgentInput struct {
	Alias        string
	ModelID      string
	Label        *string
	SystemPrompt *string
	Prelude      *string
	RateLimits   *config.WorkspaceAgentRateLimits
	Default      bool
	Enabled      bool
}

type SystemWorkspaceAgentSearch struct {
	WorkspaceID string
	Name        string
	Cursor      string
	Limit       int
}

func (store *Store) SystemWorkspaceAgentsSearch(ctx context.Context, search SystemWorkspaceAgentSearch) (error, []SystemWorkspaceAgent, string) {
	if !typed_id.Valid(typed_id.Workspace, search.WorkspaceID) {
		return nil, nil, ""
	}
	if search.Limit < 1 || search.Limit > 100 {
		return fmt.Errorf("search system workspace agents: limit must be between 1 and 100"), nil, ""
	}
	if search.Cursor != "" && !typed_id.Valid(typed_id.WorkspaceAgent, search.Cursor) {
		return fmt.Errorf("search system workspace agents: cursor is invalid"), nil, ""
	}
	query := strings.TrimSpace(search.Name)
	if len(query) > 256 {
		return fmt.Errorf("search system workspace agents: query is too long"), nil, ""
	}
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query) + "%"
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT agents.id, workspace_id, agents.alias, model_id, agents.revision, label, system_prompt, prelude, rate_limits, "default", agents.enabled, models.alias
		FROM gatehouse_workspace_agents AS agents
		JOIN gatehouse_agent_models AS models ON models.id = agents.model_id
		WHERE workspace_id = `+placeholder(1)+`
			AND (
				LOWER(agents.alias) LIKE LOWER(`+placeholder(2)+`) ESCAPE '\'
				OR LOWER(COALESCE(label, '')) LIKE LOWER(`+placeholder(3)+`) ESCAPE '\'
				OR LOWER(models.alias || ' ' || model_id) LIKE LOWER(`+placeholder(4)+`) ESCAPE '\'
				OR LOWER(agents.id) LIKE LOWER(`+placeholder(5)+`) ESCAPE '\'
			)
			AND (`+placeholder(6)+` = '' OR agents.id < `+placeholder(7)+`)
		ORDER BY agents.id DESC
		LIMIT `+placeholder(8)+`
	`, search.WorkspaceID, pattern, pattern, pattern, pattern, search.Cursor, search.Cursor, search.Limit+1)
	if err != nil {
		return fmt.Errorf("search system workspace agents: %w", err), nil, ""
	}
	defer rows.Close()
	agents := []SystemWorkspaceAgent{}
	for rows.Next() {
		var modelAlias string
		agent, err := scanSystemWorkspaceAgent(rows, &modelAlias)
		if err != nil {
			return err, nil, ""
		}
		agent.ModelAlias = &modelAlias
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate system workspace agents: %w", err), nil, ""
	}
	if len(agents) <= search.Limit {
		return nil, agents, ""
	}
	nextCursor := agents[search.Limit-1].ID
	return nil, agents[:search.Limit], nextCursor
}

func (store *Store) SystemAgentProvidersGet(ctx context.Context) (error, []SystemAgentProvider) {
	rows, err := store.QueryContext(ctx, `
		SELECT id, alias, revision, protocol, endpoint, keychain_id, keychain_version, credential IS NOT NULL, enabled, credential_metadata
		FROM gatehouse_agent_providers
		ORDER BY id`)
	if err != nil {
		return fmt.Errorf("list system agent providers: %w", err), nil
	}
	defer rows.Close()
	providers := []SystemAgentProvider{}
	for rows.Next() {
		provider, err := scanSystemAgentProvider(rows)
		if err != nil {
			return err, nil
		}
		providers = append(providers, provider)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate system agent providers: %w", err), nil
	}
	return nil, providers
}

func (store *Store) SystemAgentProviderGet(ctx context.Context, id string) (error, *SystemAgentProvider) {
	if !typed_id.Valid(typed_id.AgentProvider, id) {
		return nil, nil
	}
	provider, err := scanSystemAgentProvider(store.QueryRowContext(ctx, `
		SELECT id, alias, revision, protocol, endpoint, keychain_id, keychain_version, credential IS NOT NULL, enabled, credential_metadata
		FROM gatehouse_agent_providers WHERE id = `+keychainPlaceholder(store.kind)(1), id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("get system agent provider: %w", err), nil
	}
	return nil, &provider
}

func (store *Store) SystemAgentProviderCreate(ctx context.Context, input SystemAgentProviderInput) (error, *SystemAgentProvider) {
	input = normalizeSystemAgentProviderInput(input)
	if err := validateSystemAgentProviderInput(input, false); err != nil {
		return err, nil
	}
	id, err := typed_id.New(typed_id.AgentProvider)
	if err != nil {
		return fmt.Errorf("generate agent provider ID: %w", err), nil
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin system agent provider creation: %w", err), nil
	}
	defer transaction.Rollback()
	if err := store.systemKeychainExists(ctx, transaction, input.Keychain); err != nil {
		return err, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	var inserted string
	err = transaction.QueryRowContext(ctx, `
		INSERT INTO gatehouse_agent_providers (id, alias, revision, protocol, endpoint, keychain_id, keychain_version, credential, enabled)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, 1, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`)
		ON CONFLICT (alias) DO NOTHING RETURNING id`, id, input.Alias, input.Protocol, optionalStringValue(input.Endpoint), keychainID(input.Keychain), keychainVersion(input.Keychain), optionalStringValue(input.EncryptedCredential), input.Enabled).Scan(&inserted)
	if err == sql.ErrNoRows {
		return ErrAgentProviderExists, nil
	}
	if err != nil {
		return fmt.Errorf("insert system agent provider: %w", err), nil
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "agent_provider.create", ResourceKind: ActivityResourceKindAgentProvider, ResourceAgentProvider: &id}, []string{"sys/" + id}); err != nil {
		return fmt.Errorf("append system agent provider creation activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit system agent provider creation: %w", err), nil
	}
	return store.SystemAgentProviderGet(ctx, id)
}

func (store *Store) SystemAgentProviderUpdate(ctx context.Context, id string, expectedRevision int, input SystemAgentProviderInput) (error, *SystemAgentProvider) {
	input = normalizeSystemAgentProviderInput(input)
	if !typed_id.Valid(typed_id.AgentProvider, id) || expectedRevision < 1 {
		return nil, nil
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin system agent provider update: %w", err), nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	var alias, protocol string
	var endpoint, keychainIDValue sql.NullString
	var keychainVersionValue sql.NullInt64
	var credentialConfigured bool
	var revision int
	err = transaction.QueryRowContext(ctx, `SELECT alias, revision, protocol, endpoint, keychain_id, keychain_version, credential IS NOT NULL FROM gatehouse_agent_providers WHERE id = `+placeholder(1), id).Scan(&alias, &revision, &protocol, &endpoint, &keychainIDValue, &keychainVersionValue, &credentialConfigured)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("get system agent provider for update: %w", err), nil
	}
	if revision != expectedRevision {
		return ErrRevisionConflict, nil
	}
	if input.Alias != alias {
		return fmt.Errorf("update system agent provider: alias is immutable"), nil
	}
	keychainChanged := input.Keychain == nil || !keychainIDValue.Valid || input.Keychain.Id != keychainIDValue.String || int(keychainVersionValue.Int64) != input.Keychain.Version
	if input.Protocol == config.AgentProviderProtocolOpenAICodex && protocol == input.Protocol && credentialConfigured && keychainChanged {
		return fmt.Errorf("disconnect the provider before changing its keychain"), nil
	}
	if input.EncryptedCredential == nil && input.Protocol != "builtin" && input.Protocol != config.AgentProviderProtocolOpenAICodex && (protocol != input.Protocol || keychainChanged) {
		return fmt.Errorf("update system agent provider: credential replacement is required"), nil
	}
	if err := validateSystemAgentProviderInput(input, input.EncryptedCredential == nil); err != nil {
		return err, nil
	}
	if err := store.systemKeychainExists(ctx, transaction, input.Keychain); err != nil {
		return err, nil
	}
	// Preserve in SQL, never by writing back a ciphertext snapshot read earlier.
	preserve := input.Protocol != "builtin" && protocol == input.Protocol && input.EncryptedCredential == nil
	preserveReference := preserve && !keychainChanged
	credential := optionalStringValue(input.EncryptedCredential)
	nextRevision := revision + 1
	result, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_agent_providers
		SET revision = `+placeholder(1)+`, protocol = `+placeholder(2)+`, endpoint = `+placeholder(3)+`,
		keychain_id = CASE WHEN `+placeholder(4)+` THEN keychain_id ELSE `+placeholder(5)+` END,
		keychain_version = CASE WHEN `+placeholder(6)+` THEN keychain_version ELSE `+placeholder(7)+` END,
		credential = CASE WHEN `+placeholder(8)+` THEN credential ELSE `+placeholder(9)+` END,
		credential_metadata = CASE WHEN `+placeholder(10)+` THEN credential_metadata ELSE NULL END, enabled = `+placeholder(11)+`
		WHERE id = `+placeholder(12)+` AND revision = `+placeholder(13), nextRevision, input.Protocol, optionalStringValue(input.Endpoint), preserveReference, keychainID(input.Keychain), preserveReference, keychainVersion(input.Keychain), preserve, credential, preserve, input.Enabled, id, revision)
	if err != nil {
		return fmt.Errorf("update system agent provider: %w", err), nil
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count system agent provider update: %w", err), nil
	}
	if changed == 0 {
		return ErrRevisionConflict, nil
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "agent_provider.update", ResourceKind: ActivityResourceKindAgentProvider, ResourceAgentProvider: &id}, []string{"sys/" + id}); err != nil {
		return fmt.Errorf("append system agent provider update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit system agent provider update: %w", err), nil
	}
	return store.SystemAgentProviderGet(ctx, id)
}

func (store *Store) SystemAgentModelsGet(ctx context.Context) (error, []SystemAgentModel) {
	rows, err := store.QueryContext(ctx, `SELECT id, alias, revision, provider_id, model, parameters, compaction, max_turns, max_output_tokens, result_inline_bytes, enabled FROM gatehouse_agent_models ORDER BY id`)
	if err != nil {
		return fmt.Errorf("list system agent models: %w", err), nil
	}
	defer rows.Close()
	models := []SystemAgentModel{}
	for rows.Next() {
		value, err := scanSystemAgentModel(rows)
		if err != nil {
			return err, nil
		}
		models = append(models, value)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate system agent models: %w", err), nil
	}
	return nil, models
}

func (store *Store) SystemAgentModelGet(ctx context.Context, id string) (error, *SystemAgentModel) {
	if !typed_id.Valid(typed_id.AgentModel, id) {
		return nil, nil
	}
	value, err := scanSystemAgentModel(store.QueryRowContext(ctx, `SELECT id, alias, revision, provider_id, model, parameters, compaction, max_turns, max_output_tokens, result_inline_bytes, enabled FROM gatehouse_agent_models WHERE id = `+keychainPlaceholder(store.kind)(1), id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("get system agent model: %w", err), nil
	}
	return nil, &value
}

func (store *Store) SystemAgentModelCreate(ctx context.Context, input SystemAgentModelInput) (error, *SystemAgentModel) {
	if err := validateSystemAgentModelInput(input); err != nil {
		return err, nil
	}
	id, err := typed_id.New(typed_id.AgentModel)
	if err != nil {
		return fmt.Errorf("generate agent model ID: %w", err), nil
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin system agent model creation: %w", err), nil
	}
	defer transaction.Rollback()
	providerProtocol, err := store.systemAgentProviderProtocolGet(ctx, transaction, input.ProviderID)
	if err != nil {
		return err, nil
	}
	if err := validateSystemAgentModelProtocol(input, providerProtocol); err != nil {
		return err, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	var inserted string
	err = transaction.QueryRowContext(ctx, `INSERT INTO gatehouse_agent_models (id, alias, revision, provider_id, model, parameters, compaction, max_turns, max_output_tokens, result_inline_bytes, enabled) VALUES (`+placeholder(1)+`, `+placeholder(2)+`, 1, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`) ON CONFLICT (alias) DO NOTHING RETURNING id`, id, input.Alias, input.ProviderID, input.Model, input.Parameters, input.Compaction, input.MaxTurns, input.MaxOutputTokens, agentModelResultInlineBytes(input), input.Enabled).Scan(&inserted)
	if err == sql.ErrNoRows {
		return ErrAgentModelExists, nil
	}
	if err != nil {
		return fmt.Errorf("insert system agent model: %w", err), nil
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "agent_model.create", ResourceKind: ActivityResourceKindAgentModel, ResourceAgentModel: &id}, []string{"sys/" + id}); err != nil {
		return fmt.Errorf("append system agent model creation activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit system agent model creation: %w", err), nil
	}
	return store.SystemAgentModelGet(ctx, id)
}

func (store *Store) SystemAgentModelUpdate(ctx context.Context, id string, expectedRevision int, input SystemAgentModelInput) (error, *SystemAgentModel) {
	if !typed_id.Valid(typed_id.AgentModel, id) || expectedRevision < 1 {
		return nil, nil
	}
	if err := validateSystemAgentModelInput(input); err != nil {
		return err, nil
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin system agent model update: %w", err), nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	var alias string
	var revision int
	err = transaction.QueryRowContext(ctx, `SELECT alias, revision FROM gatehouse_agent_models WHERE id = `+placeholder(1), id).Scan(&alias, &revision)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("get system agent model for update: %w", err), nil
	}
	if revision != expectedRevision {
		return ErrRevisionConflict, nil
	}
	if input.Alias != alias {
		return fmt.Errorf("update system agent model: alias is immutable"), nil
	}
	providerProtocol, err := store.systemAgentProviderProtocolGet(ctx, transaction, input.ProviderID)
	if err != nil {
		return err, nil
	}
	if err := validateSystemAgentModelProtocol(input, providerProtocol); err != nil {
		return err, nil
	}
	result, err := transaction.ExecContext(ctx, `UPDATE gatehouse_agent_models SET revision = `+placeholder(1)+`, provider_id = `+placeholder(2)+`, model = `+placeholder(3)+`, parameters = `+placeholder(4)+`, compaction = `+placeholder(5)+`, max_turns = `+placeholder(6)+`, max_output_tokens = `+placeholder(7)+`, result_inline_bytes = `+placeholder(8)+`, enabled = `+placeholder(9)+` WHERE id = `+placeholder(10)+` AND revision = `+placeholder(11), revision+1, input.ProviderID, input.Model, input.Parameters, input.Compaction, input.MaxTurns, input.MaxOutputTokens, agentModelResultInlineBytes(input), input.Enabled, id, revision)
	if err != nil {
		return fmt.Errorf("update system agent model: %w", err), nil
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count system agent model update: %w", err), nil
	}
	if changed == 0 {
		return ErrRevisionConflict, nil
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "agent_model.update", ResourceKind: ActivityResourceKindAgentModel, ResourceAgentModel: &id}, []string{"sys/" + id}); err != nil {
		return fmt.Errorf("append system agent model update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit system agent model update: %w", err), nil
	}
	return store.SystemAgentModelGet(ctx, id)
}

func (store *Store) SystemWorkspaceAgentGet(ctx context.Context, workspaceID, bindingID string) (error, *SystemWorkspaceAgent) {
	if !typed_id.Valid(typed_id.Workspace, workspaceID) || !typed_id.Valid(typed_id.WorkspaceAgent, bindingID) {
		return nil, nil
	}
	binding, err := scanSystemWorkspaceAgent(store.QueryRowContext(ctx, `SELECT id, workspace_id, alias, model_id, revision, label, system_prompt, prelude, rate_limits, "default", enabled FROM gatehouse_workspace_agents WHERE workspace_id = `+keychainPlaceholder(store.kind)(1)+` AND id = `+keychainPlaceholder(store.kind)(2), workspaceID, bindingID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("get system workspace agent: %w", err), nil
	}
	return nil, &binding
}

func (store *Store) SystemWorkspaceAgentCreate(ctx context.Context, workspaceID string, input SystemWorkspaceAgentInput) (error, *SystemWorkspaceAgent) {
	if !typed_id.Valid(typed_id.Workspace, workspaceID) || !typed_id.Valid(typed_id.AgentModel, input.ModelID) || !workspaceAgentAlias.MatchString(input.Alias) {
		return nil, nil
	}
	rateLimits, err := workspaceAgentRateLimitsValue(input.RateLimits)
	if err != nil {
		return err, nil
	}
	id, err := typed_id.New(typed_id.WorkspaceAgent)
	if err != nil {
		return fmt.Errorf("generate workspace agent ID: %w", err), nil
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin system workspace agent creation: %w", err), nil
	}
	defer transaction.Rollback()
	if err := store.systemWorkspaceAndModelExist(ctx, transaction, workspaceID, input.ModelID); err != nil {
		return err, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	isDefault := input.Default && input.Enabled
	if isDefault {
		if _, err := transaction.ExecContext(ctx, `UPDATE gatehouse_workspace_agents SET "default" = FALSE WHERE workspace_id = `+placeholder(1)+` AND "default" = TRUE`, workspaceID); err != nil {
			return fmt.Errorf("clear system workspace agent default: %w", err), nil
		}
	}
	result, err := transaction.ExecContext(ctx, `INSERT INTO gatehouse_workspace_agents (id, workspace_id, alias, model_id, revision, label, system_prompt, prelude, rate_limits, "default", enabled) VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, 1, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`) ON CONFLICT (workspace_id, alias) DO NOTHING`, id, workspaceID, input.Alias, input.ModelID, optionalStringValue(input.Label), optionalStringValue(input.SystemPrompt), optionalStringValue(input.Prelude), rateLimits, isDefault, input.Enabled)
	if err != nil {
		return fmt.Errorf("insert system workspace agent: %w", err), nil
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count system workspace agent creation: %w", err), nil
	}
	if count == 0 {
		return ErrWorkspaceAgentExists, nil
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, workspaceAgentActivity("workspace_agent.create", workspaceID, id), []string{workspaceID}); err != nil {
		return fmt.Errorf("append system workspace agent creation activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit system workspace agent creation: %w", err), nil
	}
	return store.SystemWorkspaceAgentGet(ctx, workspaceID, id)
}

func (store *Store) SystemWorkspaceAgentUpdate(ctx context.Context, workspaceID, bindingID string, expectedRevision int, input SystemWorkspaceAgentInput) (error, *SystemWorkspaceAgent) {
	if !typed_id.Valid(typed_id.Workspace, workspaceID) || !typed_id.Valid(typed_id.WorkspaceAgent, bindingID) || !typed_id.Valid(typed_id.AgentModel, input.ModelID) || expectedRevision < 1 {
		return nil, nil
	}
	if input.Alias != "" {
		return fmt.Errorf("update system workspace agent: alias is immutable"), nil
	}
	rateLimits, err := workspaceAgentRateLimitsValue(input.RateLimits)
	if err != nil {
		return err, nil
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin system workspace agent update: %w", err), nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	var revision int
	err = transaction.QueryRowContext(ctx, `SELECT revision FROM gatehouse_workspace_agents WHERE workspace_id = `+placeholder(1)+` AND id = `+placeholder(2), workspaceID, bindingID).Scan(&revision)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("get system workspace agent for update: %w", err), nil
	}
	if revision != expectedRevision {
		return ErrRevisionConflict, nil
	}
	if err := store.systemWorkspaceAndModelExist(ctx, transaction, workspaceID, input.ModelID); err != nil {
		return err, nil
	}
	isDefault := input.Default && input.Enabled
	if isDefault {
		if _, err := transaction.ExecContext(ctx, `UPDATE gatehouse_workspace_agents SET "default" = FALSE WHERE workspace_id = `+placeholder(1)+` AND id <> `+placeholder(2)+` AND "default" = TRUE`, workspaceID, bindingID); err != nil {
			return fmt.Errorf("clear system workspace agent default: %w", err), nil
		}
	}
	result, err := transaction.ExecContext(ctx, `UPDATE gatehouse_workspace_agents SET revision = `+placeholder(1)+`, model_id = `+placeholder(2)+`, label = `+placeholder(3)+`, system_prompt = `+placeholder(4)+`, prelude = `+placeholder(5)+`, rate_limits = `+placeholder(6)+`, "default" = `+placeholder(7)+`, enabled = `+placeholder(8)+` WHERE workspace_id = `+placeholder(9)+` AND id = `+placeholder(10)+` AND revision = `+placeholder(11), revision+1, input.ModelID, optionalStringValue(input.Label), optionalStringValue(input.SystemPrompt), optionalStringValue(input.Prelude), rateLimits, isDefault, input.Enabled, workspaceID, bindingID, revision)
	if err != nil {
		return fmt.Errorf("update system workspace agent: %w", err), nil
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count system workspace agent update: %w", err), nil
	}
	if count == 0 {
		return ErrRevisionConflict, nil
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, workspaceAgentActivity("workspace_agent.update", workspaceID, bindingID), []string{workspaceID}); err != nil {
		return fmt.Errorf("append system workspace agent update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit system workspace agent update: %w", err), nil
	}
	return store.SystemWorkspaceAgentGet(ctx, workspaceID, bindingID)
}

func scanSystemAgentProvider(row interface{ Scan(...any) error }, extra ...any) (SystemAgentProvider, error) {
	var value SystemAgentProvider
	var endpoint, keychainIDValue sql.NullString
	var keychainVersionValue sql.NullInt64
	var metadata sql.NullString
	targets := []any{&value.ID, &value.Alias, &value.Revision, &value.Protocol, &endpoint, &keychainIDValue, &keychainVersionValue, &value.CredentialConfigured, &value.Enabled, &metadata}
	if err := row.Scan(append(targets, extra...)...); err != nil {
		return SystemAgentProvider{}, err
	}
	if endpoint.Valid {
		value.Endpoint = &endpoint.String
	}
	if keychainIDValue.Valid && keychainVersionValue.Valid {
		value.Keychain = &model.KeychainRef{Id: keychainIDValue.String, Version: int(keychainVersionValue.Int64)}
	}
	if metadata.Valid {
		var account SystemAgentProviderAccount
		if err := json.Unmarshal([]byte(metadata.String), &account); err != nil {
			return SystemAgentProvider{}, fmt.Errorf("decode provider account metadata")
		}
		value.Account = &account
	}
	return value, nil
}

func scanSystemAgentModel(row interface{ Scan(...any) error }) (SystemAgentModel, error) {
	var value SystemAgentModel
	if err := row.Scan(&value.ID, &value.Alias, &value.Revision, &value.ProviderID, &value.Model, &value.Parameters, &value.Compaction, &value.MaxTurns, &value.MaxOutputTokens, &value.ResultInlineBytes, &value.Enabled); err != nil {
		return SystemAgentModel{}, err
	}
	return value, nil
}

func scanSystemWorkspaceAgent(row interface{ Scan(...any) error }, extra ...*string) (SystemWorkspaceAgent, error) {
	var value SystemWorkspaceAgent
	var label, prompt, prelude, rateLimits sql.NullString
	targets := []any{&value.ID, &value.WorkspaceID, &value.Alias, &value.ModelID, &value.Revision, &label, &prompt, &prelude, &rateLimits, &value.Default, &value.Enabled}
	for _, target := range extra {
		targets = append(targets, target)
	}
	if err := row.Scan(targets...); err != nil {
		return SystemWorkspaceAgent{}, err
	}
	if label.Valid {
		value.Label = &label.String
	}
	if prompt.Valid {
		value.SystemPrompt = &prompt.String
	}
	if prelude.Valid {
		value.Prelude = &prelude.String
	}
	if rateLimits.Valid {
		limits := config.WorkspaceAgentRateLimits{}
		if err := json.Unmarshal([]byte(rateLimits.String), &limits); err != nil {
			return SystemWorkspaceAgent{}, fmt.Errorf("decode workspace agent rate limits: %w", err)
		}
		value.RateLimits = &limits
	}
	return value, nil
}

func normalizeSystemAgentProviderInput(input SystemAgentProviderInput) SystemAgentProviderInput {
	if input.Protocol == config.AgentProviderProtocolOpenAICodex && input.Endpoint == nil {
		endpoint := config.OpenAICodexEndpoint
		input.Endpoint = &endpoint
	}
	return input
}

func validateSystemAgentProviderInput(input SystemAgentProviderInput, credentialOptional bool) error {
	if !administrationAlias.MatchString(input.Alias) {
		return fmt.Errorf("system agent provider alias is invalid")
	}
	switch input.Protocol {
	case "builtin":
		if input.Endpoint != nil || input.Keychain != nil || input.EncryptedCredential != nil {
			return fmt.Errorf("builtin system agent provider credentials are invalid")
		}
	case "openai-chat-completions", "openai-responses":
		if input.Endpoint == nil || input.Keychain == nil || (!credentialOptional && input.EncryptedCredential == nil) {
			return fmt.Errorf("system agent provider credentials are required")
		}
		parsed, err := url.ParseRequestURI(*input.Endpoint)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("system agent provider endpoint is invalid")
		}
		if !administrationAlias.MatchString(input.Keychain.Id) || input.Keychain.Version < 1 {
			return fmt.Errorf("system agent provider keychain is invalid")
		}
	case config.AgentProviderProtocolOpenAICodex:
		if input.Endpoint == nil || *input.Endpoint != config.OpenAICodexEndpoint || input.Keychain == nil || !administrationAlias.MatchString(input.Keychain.Id) || input.Keychain.Version < 1 {
			return fmt.Errorf("openai-codex requires its fixed endpoint and an encryption keychain")
		}
		if input.EncryptedCredential != nil {
			return fmt.Errorf("openai-codex credentials must be installed through login")
		}
	default:
		return fmt.Errorf("system agent provider protocol is invalid")
	}
	if input.EncryptedCredential != nil && strings.TrimSpace(*input.EncryptedCredential) == "" {
		return fmt.Errorf("system agent provider credential is invalid")
	}
	return nil
}

func validateSystemAgentModelInput(input SystemAgentModelInput) error {
	if agentModelResultInlineBytes(input) <= 0 {
		return fmt.Errorf("agent model result_inline_bytes must be positive")
	}
	if !administrationAlias.MatchString(input.Alias) || !typed_id.Valid(typed_id.AgentProvider, input.ProviderID) || strings.TrimSpace(input.Model) == "" || input.MaxTurns < 1 || input.MaxOutputTokens < 1 {
		return fmt.Errorf("system agent model is invalid")
	}
	var parameters map[string]any
	if err := json.Unmarshal([]byte(input.Parameters), &parameters); err != nil || parameters == nil {
		return fmt.Errorf("system agent model parameters are invalid")
	}
	var compaction struct {
		Algorithm    string `json:"algorithm"`
		HistoryBytes int    `json:"history_bytes"`
		BufferBytes  int    `json:"buffer_bytes"`
	}
	if err := json.Unmarshal([]byte(input.Compaction), &compaction); err != nil || compaction.Algorithm != "mcmtr" || compaction.HistoryBytes < 1 || compaction.BufferBytes < 1 || compaction.BufferBytes > compaction.HistoryBytes/2 {
		return fmt.Errorf("system agent model compaction is invalid")
	}
	return nil
}

func agentModelResultInlineBytes(input SystemAgentModelInput) int {
	if input.ResultInlineBytes == nil {
		return config.DefaultAgentModelResultInlineBytes
	}
	return *input.ResultInlineBytes
}

func workspaceAgentRateLimitsValue(limits *config.WorkspaceAgentRateLimits) (any, error) {
	if limits == nil || limits.Empty() {
		return nil, nil
	}
	if err := config.ValidateWorkspaceAgentRateLimits(limits); err != nil {
		return nil, fmt.Errorf("system workspace agent rate limits are invalid: %w", err)
	}
	encoded, err := json.Marshal(limits)
	if err != nil {
		return nil, fmt.Errorf("encode system workspace agent rate limits: %w", err)
	}
	return string(encoded), nil
}

func validateSystemAgentModelProtocol(input SystemAgentModelInput, protocol string) error {
	parameters := map[string]any{}
	if err := json.Unmarshal([]byte(input.Parameters), &parameters); err != nil {
		return fmt.Errorf("system agent model parameters are invalid")
	}
	if protocol == "builtin" {
		text, ok := parameters["text"].(string)
		if input.Model != "dummy.fixed-reply" || !ok || strings.TrimSpace(text) == "" || len(parameters) != 1 {
			return fmt.Errorf("system builtin agent model is invalid")
		}
		return nil
	}
	if len(parameters) == 0 {
		return nil
	}
	effort, ok := parameters["reasoning_effort"].(string)
	if !ok || len(parameters) != 1 {
		return fmt.Errorf("system agent model parameters are invalid")
	}
	valid := map[string]bool{"none": true, "low": true, "medium": true, "high": true, "xhigh": true, "max": true}
	if !valid[effort] || (protocol == "openai-chat-completions" && effort != "none") {
		return fmt.Errorf("system agent model reasoning effort is invalid")
	}
	return nil
}

func (store *Store) systemKeychainExists(ctx context.Context, transaction *sql.Tx, keychain *model.KeychainRef) error {
	if keychain == nil {
		return nil
	}
	var id string
	err := transaction.QueryRowContext(ctx, `SELECT id FROM gatehouse_keychains WHERE id = `+keychainPlaceholder(store.kind)(1)+` AND version = `+keychainPlaceholder(store.kind)(2)+` AND enabled = TRUE`, keychain.Id, keychain.Version).Scan(&id)
	if err == sql.ErrNoRows {
		return ErrAdminDependencyMissing
	}
	if err != nil {
		return fmt.Errorf("get system keychain: %w", err)
	}
	return nil
}

func (store *Store) systemAgentProviderProtocolGet(ctx context.Context, transaction *sql.Tx, id string) (string, error) {
	if !typed_id.Valid(typed_id.AgentProvider, id) {
		return "", ErrAdminDependencyMissing
	}
	var protocol string
	err := transaction.QueryRowContext(ctx, `SELECT protocol FROM gatehouse_agent_providers WHERE id = `+keychainPlaceholder(store.kind)(1), id).Scan(&protocol)
	if err == sql.ErrNoRows {
		return "", ErrAdminDependencyMissing
	}
	if err != nil {
		return "", fmt.Errorf("get system agent provider: %w", err)
	}
	return protocol, nil
}

func (store *Store) systemWorkspaceAndModelExist(ctx context.Context, transaction *sql.Tx, workspaceID, modelID string) error {
	placeholder := keychainPlaceholder(store.kind)
	var workspace string
	err := transaction.QueryRowContext(ctx, `SELECT id FROM gatehouse_workspaces WHERE id = `+placeholder(1), workspaceID).Scan(&workspace)
	if err == sql.ErrNoRows {
		return ErrAdminDependencyMissing
	}
	if err != nil {
		return fmt.Errorf("get system workspace: %w", err)
	}
	var modelIDValue string
	err = transaction.QueryRowContext(ctx, `SELECT id FROM gatehouse_agent_models WHERE id = `+placeholder(1), modelID).Scan(&modelIDValue)
	if err == sql.ErrNoRows {
		return ErrAdminDependencyMissing
	}
	if err != nil {
		return fmt.Errorf("get system agent model: %w", err)
	}
	return nil
}

func workspaceAgentActivity(event, workspaceID, bindingID string) model.ActivityEvent {
	return model.ActivityEvent{Event: event, ResourceKind: ActivityResourceKindWorkspaceAgent, ResourceWorkspaceAgentWorkspace: &workspaceID, ResourceWorkspaceAgentId: &bindingID}
}

func optionalStringValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
func keychainID(value *model.KeychainRef) any {
	if value == nil {
		return nil
	}
	return value.Id
}
func keychainVersion(value *model.KeychainRef) any {
	if value == nil {
		return nil
	}
	return value.Version
}
