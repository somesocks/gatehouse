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

type SystemAgentProvider struct {
	ID                  string
	Alias               string
	Revision            int
	Protocol            string
	BaseURL             *string
	Keychain            *model.KeychainRef
	CredentialConfigured bool
	Enabled             bool
}

type SystemAgentProviderInput struct {
	Alias            string
	Protocol         string
	BaseURL          *string
	Keychain         *model.KeychainRef
	EncryptedAPIKey  *string
	Enabled          bool
}

type SystemAgentModel struct {
	ID              string
	Alias           string
	Revision        int
	ProviderID      string
	Model           string
	Parameters      string
	Compaction      string
	MaxTurns        int
	MaxOutputTokens int
	Enabled         bool
}

type SystemAgentModelInput struct {
	Alias           string
	ProviderID      string
	Model           string
	Parameters      string
	Compaction      string
	MaxTurns        int
	MaxOutputTokens int
	Enabled         bool
}

type SystemWorkspaceAgent struct {
	WorkspaceID  string
	ModelID      string
	Revision     int
	Priority     int
	Label        *string
	SystemPrompt *string
	Enabled      bool
}

type SystemWorkspaceAgentInput struct {
	Priority     int
	Label        *string
	SystemPrompt *string
	Enabled      bool
}

func (store *Store) SystemAgentProvidersGet(ctx context.Context) (error, []SystemAgentProvider) {
	rows, err := store.QueryContext(ctx, `
		SELECT id, alias, revision, protocol, base_url, keychain_id, keychain_version, api_key IS NOT NULL, enabled
		FROM gatehouse_agent_providers
		ORDER BY id`)
	if err != nil { return fmt.Errorf("list system agent providers: %w", err), nil }
	defer rows.Close()
	providers := []SystemAgentProvider{}
	for rows.Next() {
		provider, err := scanSystemAgentProvider(rows)
		if err != nil { return err, nil }
		providers = append(providers, provider)
	}
	if err := rows.Err(); err != nil { return fmt.Errorf("iterate system agent providers: %w", err), nil }
	return nil, providers
}

func (store *Store) SystemAgentProviderGet(ctx context.Context, id string) (error, *SystemAgentProvider) {
	if !typed_id.Valid(typed_id.AgentProvider, id) { return nil, nil }
	provider, err := scanSystemAgentProvider(store.QueryRowContext(ctx, `
		SELECT id, alias, revision, protocol, base_url, keychain_id, keychain_version, api_key IS NOT NULL, enabled
		FROM gatehouse_agent_providers WHERE id = `+keychainPlaceholder(store.kind)(1), id))
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return fmt.Errorf("get system agent provider: %w", err), nil }
	return nil, &provider
}

func (store *Store) SystemAgentProviderCreate(ctx context.Context, input SystemAgentProviderInput) (error, *SystemAgentProvider) {
	if err := validateSystemAgentProviderInput(input, false); err != nil { return err, nil }
	id, err := typed_id.New(typed_id.AgentProvider)
	if err != nil { return fmt.Errorf("generate agent provider ID: %w", err), nil }
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin system agent provider creation: %w", err), nil }
	defer transaction.Rollback()
	if err := store.systemKeychainExists(ctx, transaction, input.Keychain); err != nil { return err, nil }
	placeholder := keychainPlaceholder(store.kind)
	var inserted string
	err = transaction.QueryRowContext(ctx, `
		INSERT INTO gatehouse_agent_providers (id, alias, revision, protocol, base_url, keychain_id, keychain_version, api_key, enabled)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, 1, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`)
		ON CONFLICT (alias) DO NOTHING RETURNING id`, id, input.Alias, input.Protocol, optionalStringValue(input.BaseURL), keychainID(input.Keychain), keychainVersion(input.Keychain), optionalStringValue(input.EncryptedAPIKey), input.Enabled).Scan(&inserted)
	if err == sql.ErrNoRows { return ErrAgentProviderExists, nil }
	if err != nil { return fmt.Errorf("insert system agent provider: %w", err), nil }
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "agent_provider.create", ResourceKind: ActivityResourceKindAgentProvider, ResourceAgentProvider: &id}, []string{"sys/" + id}); err != nil {
		return fmt.Errorf("append system agent provider creation activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil { return fmt.Errorf("commit system agent provider creation: %w", err), nil }
	return store.SystemAgentProviderGet(ctx, id)
}

func (store *Store) SystemAgentProviderUpdate(ctx context.Context, id string, expectedRevision int, input SystemAgentProviderInput) (error, *SystemAgentProvider) {
	if !typed_id.Valid(typed_id.AgentProvider, id) || expectedRevision < 1 { return nil, nil }
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin system agent provider update: %w", err), nil }
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	var alias, protocol string
	var baseURL, keychainIDValue sql.NullString
	var keychainVersionValue sql.NullInt64
	var encrypted sql.NullString
	var revision int
	err = transaction.QueryRowContext(ctx, `SELECT alias, revision, protocol, base_url, keychain_id, keychain_version, api_key FROM gatehouse_agent_providers WHERE id = `+placeholder(1), id).Scan(&alias, &revision, &protocol, &baseURL, &keychainIDValue, &keychainVersionValue, &encrypted)
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return fmt.Errorf("get system agent provider for update: %w", err), nil }
	if revision != expectedRevision { return ErrRevisionConflict, nil }
	if input.Alias != alias { return fmt.Errorf("update system agent provider: alias is immutable"), nil }
	if input.EncryptedAPIKey == nil && input.Protocol != "builtin" && (protocol != input.Protocol || input.Keychain == nil || !keychainIDValue.Valid || input.Keychain.Id != keychainIDValue.String || int(keychainVersionValue.Int64) != input.Keychain.Version) {
		return fmt.Errorf("update system agent provider: credential replacement is required"), nil
	}
	if err := validateSystemAgentProviderInput(input, input.EncryptedAPIKey == nil); err != nil { return err, nil }
	if err := store.systemKeychainExists(ctx, transaction, input.Keychain); err != nil { return err, nil }
	credential := any(nil)
	if encrypted.Valid { credential = encrypted.String }
	if input.EncryptedAPIKey != nil { credential = *input.EncryptedAPIKey }
	if input.Protocol == "builtin" { credential = nil }
	nextRevision := revision + 1
	result, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_agent_providers
		SET revision = `+placeholder(1)+`, protocol = `+placeholder(2)+`, base_url = `+placeholder(3)+`, keychain_id = `+placeholder(4)+`, keychain_version = `+placeholder(5)+`, api_key = `+placeholder(6)+`, enabled = `+placeholder(7)+`
		WHERE id = `+placeholder(8)+` AND revision = `+placeholder(9), nextRevision, input.Protocol, optionalStringValue(input.BaseURL), keychainID(input.Keychain), keychainVersion(input.Keychain), credential, input.Enabled, id, revision)
	if err != nil { return fmt.Errorf("update system agent provider: %w", err), nil }
	changed, err := result.RowsAffected()
	if err != nil { return fmt.Errorf("count system agent provider update: %w", err), nil }
	if changed == 0 { return ErrRevisionConflict, nil }
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "agent_provider.update", ResourceKind: ActivityResourceKindAgentProvider, ResourceAgentProvider: &id}, []string{"sys/" + id}); err != nil {
		return fmt.Errorf("append system agent provider update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil { return fmt.Errorf("commit system agent provider update: %w", err), nil }
	return store.SystemAgentProviderGet(ctx, id)
}

func (store *Store) SystemAgentModelsGet(ctx context.Context) (error, []SystemAgentModel) {
	rows, err := store.QueryContext(ctx, `SELECT id, alias, revision, provider_id, model, parameters, compaction, max_turns, max_output_tokens, enabled FROM gatehouse_agent_models ORDER BY id`)
	if err != nil { return fmt.Errorf("list system agent models: %w", err), nil }
	defer rows.Close()
	models := []SystemAgentModel{}
	for rows.Next() { value, err := scanSystemAgentModel(rows); if err != nil { return err, nil }; models = append(models, value) }
	if err := rows.Err(); err != nil { return fmt.Errorf("iterate system agent models: %w", err), nil }
	return nil, models
}

func (store *Store) SystemAgentModelGet(ctx context.Context, id string) (error, *SystemAgentModel) {
	if !typed_id.Valid(typed_id.AgentModel, id) { return nil, nil }
	value, err := scanSystemAgentModel(store.QueryRowContext(ctx, `SELECT id, alias, revision, provider_id, model, parameters, compaction, max_turns, max_output_tokens, enabled FROM gatehouse_agent_models WHERE id = `+keychainPlaceholder(store.kind)(1), id))
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return fmt.Errorf("get system agent model: %w", err), nil }
	return nil, &value
}

func (store *Store) SystemAgentModelCreate(ctx context.Context, input SystemAgentModelInput) (error, *SystemAgentModel) {
	if err := validateSystemAgentModelInput(input); err != nil { return err, nil }
	id, err := typed_id.New(typed_id.AgentModel)
	if err != nil { return fmt.Errorf("generate agent model ID: %w", err), nil }
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin system agent model creation: %w", err), nil }
	defer transaction.Rollback()
	providerProtocol, err := store.systemAgentProviderProtocolGet(ctx, transaction, input.ProviderID)
	if err != nil { return err, nil }
	if err := validateSystemAgentModelProtocol(input, providerProtocol); err != nil { return err, nil }
	placeholder := keychainPlaceholder(store.kind)
	var inserted string
	err = transaction.QueryRowContext(ctx, `INSERT INTO gatehouse_agent_models (id, alias, revision, provider_id, model, parameters, compaction, max_turns, max_output_tokens, enabled) VALUES (`+placeholder(1)+`, `+placeholder(2)+`, 1, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`) ON CONFLICT (alias) DO NOTHING RETURNING id`, id, input.Alias, input.ProviderID, input.Model, input.Parameters, input.Compaction, input.MaxTurns, input.MaxOutputTokens, input.Enabled).Scan(&inserted)
	if err == sql.ErrNoRows { return ErrAgentModelExists, nil }
	if err != nil { return fmt.Errorf("insert system agent model: %w", err), nil }
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "agent_model.create", ResourceKind: ActivityResourceKindAgentModel, ResourceAgentModel: &id}, []string{"sys/" + id}); err != nil { return fmt.Errorf("append system agent model creation activity: %w", err), nil }
	if err := transaction.Commit(); err != nil { return fmt.Errorf("commit system agent model creation: %w", err), nil }
	return store.SystemAgentModelGet(ctx, id)
}

func (store *Store) SystemAgentModelUpdate(ctx context.Context, id string, expectedRevision int, input SystemAgentModelInput) (error, *SystemAgentModel) {
	if !typed_id.Valid(typed_id.AgentModel, id) || expectedRevision < 1 { return nil, nil }
	if err := validateSystemAgentModelInput(input); err != nil { return err, nil }
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin system agent model update: %w", err), nil }
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	var alias string
	var revision int
	err = transaction.QueryRowContext(ctx, `SELECT alias, revision FROM gatehouse_agent_models WHERE id = `+placeholder(1), id).Scan(&alias, &revision)
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return fmt.Errorf("get system agent model for update: %w", err), nil }
	if revision != expectedRevision { return ErrRevisionConflict, nil }
	if input.Alias != alias { return fmt.Errorf("update system agent model: alias is immutable"), nil }
	providerProtocol, err := store.systemAgentProviderProtocolGet(ctx, transaction, input.ProviderID)
	if err != nil { return err, nil }
	if err := validateSystemAgentModelProtocol(input, providerProtocol); err != nil { return err, nil }
	result, err := transaction.ExecContext(ctx, `UPDATE gatehouse_agent_models SET revision = `+placeholder(1)+`, provider_id = `+placeholder(2)+`, model = `+placeholder(3)+`, parameters = `+placeholder(4)+`, compaction = `+placeholder(5)+`, max_turns = `+placeholder(6)+`, max_output_tokens = `+placeholder(7)+`, enabled = `+placeholder(8)+` WHERE id = `+placeholder(9)+` AND revision = `+placeholder(10), revision+1, input.ProviderID, input.Model, input.Parameters, input.Compaction, input.MaxTurns, input.MaxOutputTokens, input.Enabled, id, revision)
	if err != nil { return fmt.Errorf("update system agent model: %w", err), nil }
	changed, err := result.RowsAffected(); if err != nil { return fmt.Errorf("count system agent model update: %w", err), nil }; if changed == 0 { return ErrRevisionConflict, nil }
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "agent_model.update", ResourceKind: ActivityResourceKindAgentModel, ResourceAgentModel: &id}, []string{"sys/" + id}); err != nil { return fmt.Errorf("append system agent model update activity: %w", err), nil }
	if err := transaction.Commit(); err != nil { return fmt.Errorf("commit system agent model update: %w", err), nil }
	return store.SystemAgentModelGet(ctx, id)
}

func (store *Store) SystemWorkspaceAgentsGet(ctx context.Context) (error, []SystemWorkspaceAgent) {
	rows, err := store.QueryContext(ctx, `SELECT workspace_id, model_id, revision, priority, label, system_prompt, enabled FROM gatehouse_workspace_agents ORDER BY workspace_id, model_id`)
	if err != nil { return fmt.Errorf("list system workspace agents: %w", err), nil }
	defer rows.Close()
	bindings := []SystemWorkspaceAgent{}
	for rows.Next() { binding, err := scanSystemWorkspaceAgent(rows); if err != nil { return err, nil }; bindings = append(bindings, binding) }
	if err := rows.Err(); err != nil { return fmt.Errorf("iterate system workspace agents: %w", err), nil }
	return nil, bindings
}

func (store *Store) SystemWorkspaceAgentGet(ctx context.Context, workspaceID, modelID string) (error, *SystemWorkspaceAgent) {
	if !typed_id.Valid(typed_id.Workspace, workspaceID) || !typed_id.Valid(typed_id.AgentModel, modelID) { return nil, nil }
	binding, err := scanSystemWorkspaceAgent(store.QueryRowContext(ctx, `SELECT workspace_id, model_id, revision, priority, label, system_prompt, enabled FROM gatehouse_workspace_agents WHERE workspace_id = `+keychainPlaceholder(store.kind)(1)+` AND model_id = `+keychainPlaceholder(store.kind)(2), workspaceID, modelID))
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return fmt.Errorf("get system workspace agent: %w", err), nil }
	return nil, &binding
}

func (store *Store) SystemWorkspaceAgentCreate(ctx context.Context, workspaceID, modelID string, input SystemWorkspaceAgentInput) (error, *SystemWorkspaceAgent) {
	if !typed_id.Valid(typed_id.Workspace, workspaceID) || !typed_id.Valid(typed_id.AgentModel, modelID) || input.Priority < 1 { return nil, nil }
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin system workspace agent creation: %w", err), nil }
	defer transaction.Rollback()
	if err := store.systemWorkspaceAndModelExist(ctx, transaction, workspaceID, modelID); err != nil { return err, nil }
	placeholder := keychainPlaceholder(store.kind)
	result, err := transaction.ExecContext(ctx, `INSERT INTO gatehouse_workspace_agents (workspace_id, model_id, revision, priority, label, system_prompt, enabled) VALUES (`+placeholder(1)+`, `+placeholder(2)+`, 1, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`) ON CONFLICT (workspace_id, model_id) DO NOTHING`, workspaceID, modelID, input.Priority, optionalStringValue(input.Label), optionalStringValue(input.SystemPrompt), input.Enabled)
	if err != nil { return fmt.Errorf("insert system workspace agent: %w", err), nil }
	count, err := result.RowsAffected(); if err != nil { return fmt.Errorf("count system workspace agent creation: %w", err), nil }; if count == 0 { return ErrWorkspaceAgentExists, nil }
	if err, _ := store.ActivityEventAppend(ctx, transaction, workspaceAgentActivity("workspace_agent.create", workspaceID, modelID), []string{workspaceID}); err != nil { return fmt.Errorf("append system workspace agent creation activity: %w", err), nil }
	if err := transaction.Commit(); err != nil { return fmt.Errorf("commit system workspace agent creation: %w", err), nil }
	return store.SystemWorkspaceAgentGet(ctx, workspaceID, modelID)
}

func (store *Store) SystemWorkspaceAgentUpdate(ctx context.Context, workspaceID, modelID string, expectedRevision int, input SystemWorkspaceAgentInput) (error, *SystemWorkspaceAgent) {
	if !typed_id.Valid(typed_id.Workspace, workspaceID) || !typed_id.Valid(typed_id.AgentModel, modelID) || expectedRevision < 1 || input.Priority < 1 { return nil, nil }
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil { return fmt.Errorf("begin system workspace agent update: %w", err), nil }
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	var revision int
	err = transaction.QueryRowContext(ctx, `SELECT revision FROM gatehouse_workspace_agents WHERE workspace_id = `+placeholder(1)+` AND model_id = `+placeholder(2), workspaceID, modelID).Scan(&revision)
	if err == sql.ErrNoRows { return nil, nil }
	if err != nil { return fmt.Errorf("get system workspace agent for update: %w", err), nil }
	if revision != expectedRevision { return ErrRevisionConflict, nil }
	result, err := transaction.ExecContext(ctx, `UPDATE gatehouse_workspace_agents SET revision = `+placeholder(1)+`, priority = `+placeholder(2)+`, label = `+placeholder(3)+`, system_prompt = `+placeholder(4)+`, enabled = `+placeholder(5)+` WHERE workspace_id = `+placeholder(6)+` AND model_id = `+placeholder(7)+` AND revision = `+placeholder(8), revision+1, input.Priority, optionalStringValue(input.Label), optionalStringValue(input.SystemPrompt), input.Enabled, workspaceID, modelID, revision)
	if err != nil { return fmt.Errorf("update system workspace agent: %w", err), nil }
	count, err := result.RowsAffected(); if err != nil { return fmt.Errorf("count system workspace agent update: %w", err), nil }; if count == 0 { return ErrRevisionConflict, nil }
	if err, _ := store.ActivityEventAppend(ctx, transaction, workspaceAgentActivity("workspace_agent.update", workspaceID, modelID), []string{workspaceID}); err != nil { return fmt.Errorf("append system workspace agent update activity: %w", err), nil }
	if err := transaction.Commit(); err != nil { return fmt.Errorf("commit system workspace agent update: %w", err), nil }
	return store.SystemWorkspaceAgentGet(ctx, workspaceID, modelID)
}

func scanSystemAgentProvider(row interface { Scan(...any) error }) (SystemAgentProvider, error) {
	var value SystemAgentProvider
	var baseURL, keychainIDValue sql.NullString
	var keychainVersionValue sql.NullInt64
	if err := row.Scan(&value.ID, &value.Alias, &value.Revision, &value.Protocol, &baseURL, &keychainIDValue, &keychainVersionValue, &value.CredentialConfigured, &value.Enabled); err != nil { return SystemAgentProvider{}, err }
	if baseURL.Valid { value.BaseURL = &baseURL.String }
	if keychainIDValue.Valid && keychainVersionValue.Valid { value.Keychain = &model.KeychainRef{Id: keychainIDValue.String, Version: int(keychainVersionValue.Int64)} }
	return value, nil
}

func scanSystemAgentModel(row interface { Scan(...any) error }) (SystemAgentModel, error) {
	var value SystemAgentModel
	if err := row.Scan(&value.ID, &value.Alias, &value.Revision, &value.ProviderID, &value.Model, &value.Parameters, &value.Compaction, &value.MaxTurns, &value.MaxOutputTokens, &value.Enabled); err != nil { return SystemAgentModel{}, err }
	return value, nil
}

func scanSystemWorkspaceAgent(row interface { Scan(...any) error }) (SystemWorkspaceAgent, error) {
	var value SystemWorkspaceAgent
	var label, prompt sql.NullString
	if err := row.Scan(&value.WorkspaceID, &value.ModelID, &value.Revision, &value.Priority, &label, &prompt, &value.Enabled); err != nil { return SystemWorkspaceAgent{}, err }
	if label.Valid { value.Label = &label.String }
	if prompt.Valid { value.SystemPrompt = &prompt.String }
	return value, nil
}

func validateSystemAgentProviderInput(input SystemAgentProviderInput, credentialOptional bool) error {
	if !administrationAlias.MatchString(input.Alias) { return fmt.Errorf("system agent provider alias is invalid") }
	switch input.Protocol {
	case "builtin":
		if input.BaseURL != nil || input.Keychain != nil || input.EncryptedAPIKey != nil { return fmt.Errorf("builtin system agent provider credentials are invalid") }
	case "openai-chat-completions", "openai-responses":
		if input.BaseURL == nil || input.Keychain == nil || (!credentialOptional && input.EncryptedAPIKey == nil) { return fmt.Errorf("system agent provider credentials are required") }
		parsed, err := url.ParseRequestURI(*input.BaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" { return fmt.Errorf("system agent provider base URL is invalid") }
		if !administrationAlias.MatchString(input.Keychain.Id) || input.Keychain.Version < 1 { return fmt.Errorf("system agent provider keychain is invalid") }
	default:
		return fmt.Errorf("system agent provider protocol is invalid")
	}
	if input.EncryptedAPIKey != nil && strings.TrimSpace(*input.EncryptedAPIKey) == "" { return fmt.Errorf("system agent provider credential is invalid") }
	return nil
}

func validateSystemAgentModelInput(input SystemAgentModelInput) error {
	if !administrationAlias.MatchString(input.Alias) || !typed_id.Valid(typed_id.AgentProvider, input.ProviderID) || strings.TrimSpace(input.Model) == "" || input.MaxTurns < 1 || input.MaxOutputTokens < 1 { return fmt.Errorf("system agent model is invalid") }
	var parameters map[string]any
	if err := json.Unmarshal([]byte(input.Parameters), &parameters); err != nil || parameters == nil { return fmt.Errorf("system agent model parameters are invalid") }
	var compaction struct { Algorithm string `json:"algorithm"`; HistoryBytes int `json:"history_bytes"`; BufferBytes int `json:"buffer_bytes"` }
	if err := json.Unmarshal([]byte(input.Compaction), &compaction); err != nil || compaction.Algorithm != "mcmtr" || compaction.HistoryBytes < 1 || compaction.BufferBytes < 1 || compaction.BufferBytes > compaction.HistoryBytes/6 { return fmt.Errorf("system agent model compaction is invalid") }
	return nil
}

func validateSystemAgentModelProtocol(input SystemAgentModelInput, protocol string) error {
	parameters := map[string]any{}
	if err := json.Unmarshal([]byte(input.Parameters), &parameters); err != nil { return fmt.Errorf("system agent model parameters are invalid") }
	if protocol == "builtin" {
		text, ok := parameters["text"].(string)
		if input.Model != "dummy.fixed-reply" || !ok || strings.TrimSpace(text) == "" || len(parameters) != 1 { return fmt.Errorf("system builtin agent model is invalid") }
		return nil
	}
	if len(parameters) == 0 { return nil }
	effort, ok := parameters["reasoning_effort"].(string)
	if !ok || len(parameters) != 1 { return fmt.Errorf("system agent model parameters are invalid") }
	valid := map[string]bool{"none": true, "low": true, "medium": true, "high": true, "xhigh": true, "max": true}
	if !valid[effort] || (protocol == "openai-chat-completions" && effort != "none") { return fmt.Errorf("system agent model reasoning effort is invalid") }
	return nil
}

func (store *Store) systemKeychainExists(ctx context.Context, transaction *sql.Tx, keychain *model.KeychainRef) error {
	if keychain == nil { return nil }
	var id string
	err := transaction.QueryRowContext(ctx, `SELECT id FROM gatehouse_keychains WHERE id = `+keychainPlaceholder(store.kind)(1)+` AND version = `+keychainPlaceholder(store.kind)(2)+` AND enabled = TRUE`, keychain.Id, keychain.Version).Scan(&id)
	if err == sql.ErrNoRows { return ErrAdminDependencyMissing }
	if err != nil { return fmt.Errorf("get system keychain: %w", err) }
	return nil
}

func (store *Store) systemAgentProviderProtocolGet(ctx context.Context, transaction *sql.Tx, id string) (string, error) {
	if !typed_id.Valid(typed_id.AgentProvider, id) { return "", ErrAdminDependencyMissing }
	var protocol string
	err := transaction.QueryRowContext(ctx, `SELECT protocol FROM gatehouse_agent_providers WHERE id = `+keychainPlaceholder(store.kind)(1), id).Scan(&protocol)
	if err == sql.ErrNoRows { return "", ErrAdminDependencyMissing }
	if err != nil { return "", fmt.Errorf("get system agent provider: %w", err) }
	return protocol, nil
}

func (store *Store) systemWorkspaceAndModelExist(ctx context.Context, transaction *sql.Tx, workspaceID, modelID string) error {
	placeholder := keychainPlaceholder(store.kind)
	var workspace string
	err := transaction.QueryRowContext(ctx, `SELECT id FROM gatehouse_workspaces WHERE id = `+placeholder(1), workspaceID).Scan(&workspace)
	if err == sql.ErrNoRows { return ErrAdminDependencyMissing }
	if err != nil { return fmt.Errorf("get system workspace: %w", err) }
	var modelIDValue string
	err = transaction.QueryRowContext(ctx, `SELECT id FROM gatehouse_agent_models WHERE id = `+placeholder(1), modelID).Scan(&modelIDValue)
	if err == sql.ErrNoRows { return ErrAdminDependencyMissing }
	if err != nil { return fmt.Errorf("get system agent model: %w", err) }
	return nil
}

func workspaceAgentActivity(event, workspaceID, modelID string) model.ActivityEvent {
	return model.ActivityEvent{Event: event, ResourceKind: ActivityResourceKindWorkspaceAgent, ResourceWorkspaceAgentWorkspace: &workspaceID, ResourceWorkspaceAgentModel: &modelID}
}

func optionalStringValue(value *string) any { if value == nil { return nil }; return *value }
func keychainID(value *model.KeychainRef) any { if value == nil { return nil }; return value.Id }
func keychainVersion(value *model.KeychainRef) any { if value == nil { return nil }; return value.Version }
