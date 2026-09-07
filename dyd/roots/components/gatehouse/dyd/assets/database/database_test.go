package database_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/typed_id"
)

func TestMigrateAppliesConfiguredMigrations(t *testing.T) {
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, database := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrateState(context.Background(), database, configuration, config.State{}); err != nil {
		t.Fatal(err)
	}

	var workspaceCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM gatehouse_workspaces`).Scan(&workspaceCount); err != nil {
		t.Fatal(err)
	}
	if workspaceCount != 1 {
		t.Fatalf("workspace count = %d, want 1", workspaceCount)
	}
	for _, table := range []string{
		"gatehouse_agent_providers",
		"gatehouse_agent_models",
		"gatehouse_workspace_agents",
		"gatehouse_agent_tasks__session_event_reply",
		"gatehouse_session_approval_decisions",
		"gatehouse_storage_providers",
		"gatehouse_workspace_storage_providers",
	} {
		if _, err := database.Exec(`SELECT * FROM ` + table + ` LIMIT 0`); err != nil {
			t.Fatalf("agent table %q is unavailable: %v", table, err)
		}
	}
}

func TestSQLiteIDFunctions(t *testing.T) {
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var current string
	if err := store.QueryRowContext(ctx, `SELECT gh_id_new('act')`).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if !typed_id.Valid(typed_id.ActivityEvent, current) {
		t.Fatalf("gh_id_new() = %q, want valid activity ID", current)
	}

	const milliseconds = int64(1_700_000_000_123)
	var fixed, timestamp string
	if err := store.QueryRowContext(ctx, `SELECT gh_id_new('act', ?)`, milliseconds).Scan(&fixed); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `SELECT gh_id_timestamp(?)`, fixed).Scan(&timestamp); err != nil {
		t.Fatal(err)
	}
	wantTimestamp := time.UnixMilli(milliseconds).UTC().Format("2006-01-02T15:04:05.000Z")
	if timestamp != wantTimestamp {
		t.Fatalf("gh_id_timestamp(%q) = %q, want %q", fixed, timestamp, wantTimestamp)
	}
	if _, err := store.ExecContext(ctx, `SELECT gh_id_new('invalid-prefix')`); err == nil {
		t.Fatal("gh_id_new accepted an invalid prefix")
	}
	if _, err := store.ExecContext(ctx, `SELECT gh_id_timestamp('act_invalid')`); err == nil {
		t.Fatal("gh_id_timestamp accepted an invalid ID")
	}
	var verifiers string
	if err := store.QueryRowContext(ctx, `SELECT gh_identity_verifiers('gatehouse:alice', '[{"Value":"gh-ver:AAAA"}]')`).Scan(&verifiers); err != nil {
		t.Fatal(err)
	}
	if verifiers != `["gh-ver:AAAA"]` {
		t.Fatalf("gh_identity_verifiers() = %q, want static verifier", verifiers)
	}
}

func TestMigrateSQLiteEmitsReconciliationActivityTargets(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	state := config.State{
		Keychains: []config.Keychain{{ID: "events", Sources: []config.KeychainPassphraseSource{"env:EVENTS_KEYCHAIN"}}},
		Principals: []config.Principal{{
			Alias: "alice", Revision: 1, Enabled: true,
			Identities: []config.Identity{{
				Alias: "alice-matrix", Key: "matrix:@alice:example.org", Revision: 1, Enabled: true,
				Verifiers: []config.Verifier{{Stored: map[string]any{"kind": "matrix"}}},
			}},
		}},
		AgentProviders: []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{{
			Alias: "fallback", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Fallback reply."}`,
			MaxTurns: config.DefaultAgentModelMaxTurns, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true,
		}},
		StorageProviders: []config.StorageProvider{{Alias: "embedded", Revision: 1, Protocol: "embedded", Enabled: true}},
	}
	t.Setenv("EVENTS_KEYCHAIN", "events passphrase")
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}

	principal := principalRef(t, ctx, store, "alice")
	var identityID, providerID, modelID, storageProviderID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_identities WHERE alias = 'alice-matrix'`).Scan(&identityID); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_agent_providers WHERE alias = 'builtin'`).Scan(&providerID); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_agent_models WHERE alias = 'fallback'`).Scan(&modelID); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_storage_providers WHERE alias = 'embedded'`).Scan(&storageProviderID); err != nil {
		t.Fatal(err)
	}

	assertActivity := func(event, targetColumn, target, topic string, want int) {
		t.Helper()
		var count int
		if err := store.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM gatehouse_activity_events AS events
			JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
			WHERE events.event = ? AND events.`+targetColumn+` = ? AND topics.topic = ?
		`, event, target, topic).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Fatalf("%s activity count = %d, want %d", event, count, want)
		}
	}

	assertActivity("keychain.create", "resource_keychain_id", "events", "sys/kch/events/1", 1)
	var keychainVersion int
	if err := store.QueryRowContext(ctx, `
		SELECT resource_keychain_version
		FROM gatehouse_activity_events
		WHERE event = 'keychain.create' AND resource_keychain_id = 'events'
	`).Scan(&keychainVersion); err != nil {
		t.Fatal(err)
	}
	if keychainVersion != 1 {
		t.Fatalf("keychain activity target version = %d, want 1", keychainVersion)
	}
	assertActivity("principal.create", "resource_principal", principal.Id, principal.Id, 1)
	assertActivity("identity.create", "resource_identity", identityID, principal.Id+"/"+identityID, 1)
	assertActivity("agent_provider.create", "resource_agent_provider", providerID, "sys/"+providerID, 1)
	assertActivity("agent_model.create", "resource_agent_model", modelID, "sys/"+modelID, 1)
	assertActivity("storage_provider.create", "resource_storage_provider", storageProviderID, "sys/"+storageProviderID, 1)

	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	assertActivity("keychain.create", "resource_keychain_id", "events", "sys/kch/events/1", 1)
	assertActivity("principal.create", "resource_principal", principal.Id, principal.Id, 1)
	assertActivity("identity.create", "resource_identity", identityID, principal.Id+"/"+identityID, 1)
	assertActivity("agent_provider.create", "resource_agent_provider", providerID, "sys/"+providerID, 1)
	assertActivity("agent_model.create", "resource_agent_model", modelID, "sys/"+modelID, 1)
	assertActivity("storage_provider.create", "resource_storage_provider", storageProviderID, "sys/"+storageProviderID, 1)

	state.Principals[0].Revision = 2
	state.Principals[0].Identities[0].Revision = 2
	state.AgentProviders[0].Revision = 2
	state.AgentModels[0].Revision = 2
	state.StorageProviders[0].Revision = 2
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	assertActivity("principal.update", "resource_principal", principal.Id, principal.Id, 1)
	assertActivity("identity.update", "resource_identity", identityID, principal.Id+"/"+identityID, 1)
	assertActivity("agent_provider.update", "resource_agent_provider", providerID, "sys/"+providerID, 1)
	assertActivity("agent_model.update", "resource_agent_model", modelID, "sys/"+modelID, 1)
	assertActivity("storage_provider.update", "resource_storage_provider", storageProviderID, "sys/"+storageProviderID, 1)

	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	assertActivity("principal.update", "resource_principal", principal.Id, principal.Id, 1)
	assertActivity("identity.update", "resource_identity", identityID, principal.Id+"/"+identityID, 1)
	assertActivity("agent_provider.update", "resource_agent_provider", providerID, "sys/"+providerID, 1)
	assertActivity("agent_model.update", "resource_agent_model", modelID, "sys/"+modelID, 1)
	assertActivity("storage_provider.update", "resource_storage_provider", storageProviderID, "sys/"+storageProviderID, 1)

	state.Principals[0].Revision = 1
	state.Principals[0].Identities[0].Revision = 1
	state.AgentProviders[0].Revision = 1
	state.AgentModels[0].Revision = 1
	state.StorageProviders[0].Revision = 1
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	assertActivity("principal.update", "resource_principal", principal.Id, principal.Id, 1)
	assertActivity("identity.update", "resource_identity", identityID, principal.Id+"/"+identityID, 1)
	assertActivity("agent_provider.update", "resource_agent_provider", providerID, "sys/"+providerID, 1)
	assertActivity("agent_model.update", "resource_agent_model", modelID, "sys/"+modelID, 1)
	assertActivity("storage_provider.update", "resource_storage_provider", storageProviderID, "sys/"+storageProviderID, 1)
}

func TestMigrateReconcilesStorageProvidersAndWorkspaceBindings(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	t.Setenv("DOCUMENTS_SECRET", "secret-1")
	state := config.State{
		Keychains:  []config.Keychain{{ID: "storage", Sources: []config.KeychainPassphraseSource{"env:DOCUMENTS_KEYCHAIN"}}},
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		StorageProviders: []config.StorageProvider{
			{Alias: "embedded", Revision: 1, Protocol: "embedded", Enabled: true},
			{
				Alias: "documents", Revision: 1, Protocol: "s3", Enabled: true,
				Endpoint: stringPointer("https://s3.example.test"), Region: stringPointer("us-east-1"), Bucket: stringPointer("documents"), AccessKeyID: stringPointer("access-key"), Keychain: stringPointer("storage"), SecretKeySources: []config.StorageProviderSecretKeySource{"env:DOCUMENTS_SECRET"},
			},
		},
		WorkspaceStorageProviders: []config.WorkspaceStorageProvider{
			{WorkspaceID: "engineering", ProviderAlias: "documents", Revision: 1, Priority: 10, Enabled: true},
			{WorkspaceID: "engineering", ProviderAlias: "embedded", Revision: 1, Priority: 1, Enabled: true},
		},
	}
	t.Setenv("DOCUMENTS_KEYCHAIN", "storage passphrase")
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	var activityCount int

	var (
		protocol, endpoint, region, bucket, accessKeyID, keychainID, secret string
		keychainVersion, revision, priority                                 int
		enabled                                                             bool
	)
	if err := store.QueryRowContext(ctx, `
		SELECT protocol, endpoint, region, bucket, access_key_id, keychain_id, keychain_version, secret_access_key, revision, enabled
		FROM gatehouse_storage_providers WHERE alias = 'documents'
	`).Scan(&protocol, &endpoint, &region, &bucket, &accessKeyID, &keychainID, &keychainVersion, &secret, &revision, &enabled); err != nil {
		t.Fatal(err)
	}
	if protocol != "s3" || endpoint != "https://s3.example.test" || region != "us-east-1" || bucket != "documents" || accessKeyID != "access-key" || keychainID != "storage" || keychainVersion != 1 || secret == "secret-1" || revision != 1 || !enabled {
		t.Fatalf("documents storage provider = (%q, %q, %q, %q, %q, %q, %d, %q, %d, %t)", protocol, endpoint, region, bucket, accessKeyID, keychainID, keychainVersion, secret, revision, enabled)
	}
	err, encryptedSecret := keychain.ParseKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	if encryptedSecret.AAD != keychain.AADAlias {
		t.Fatalf("storage provider ciphertext AAD = %q, want %q", encryptedSecret.AAD, keychain.AADAlias)
	}
	var documentsID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_storage_providers WHERE alias = 'documents'`).Scan(&documentsID); err != nil {
		t.Fatal(err)
	}
	if !typed_id.Valid(typed_id.StorageProvider, documentsID) {
		t.Fatalf("documents storage provider ID = %q, want typed ID", documentsID)
	}
	if err := store.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM gatehouse_activity_events AS events
		JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
		WHERE events.event = 'workspace_storage_provider.create' AND topics.topic = ?
	`, database.ActivityTopicWorkspaceStorageProvider(workspace)).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 2 {
		t.Fatalf("workspace storage provider create activity count = %d, want 2", activityCount)
	}
	if err := store.QueryRowContext(ctx, `
		SELECT priority, enabled
		FROM gatehouse_workspace_storage_providers
		WHERE workspace = ? AND provider = (SELECT id FROM gatehouse_storage_providers WHERE alias = 'documents')
	`, workspace.Id).Scan(&priority, &enabled); err != nil {
		t.Fatal(err)
	}
	if priority != 10 || !enabled {
		t.Fatalf("documents workspace binding = (%d, %t)", priority, enabled)
	}

	state.StorageProviders[1].Revision = 2
	state.StorageProviders[1].Endpoint = stringPointer("https://s3-next.example.test")
	state.WorkspaceStorageProviders[0].Revision = 2
	state.WorkspaceStorageProviders[0].Priority = 20
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `SELECT endpoint, revision FROM gatehouse_storage_providers WHERE alias = 'documents'`).Scan(&endpoint, &revision); err != nil {
		t.Fatal(err)
	}
	if endpoint != "https://s3-next.example.test" || revision != 2 {
		t.Fatalf("updated documents storage provider = (%q, %d)", endpoint, revision)
	}
	if err := store.QueryRowContext(ctx, `SELECT priority, revision FROM gatehouse_workspace_storage_providers WHERE workspace = ? AND provider = (SELECT id FROM gatehouse_storage_providers WHERE alias = 'documents')`, workspace.Id).Scan(&priority, &revision); err != nil {
		t.Fatal(err)
	}
	if priority != 20 || revision != 2 {
		t.Fatalf("updated documents workspace binding = (%d, %d)", priority, revision)
	}
	if err := store.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM gatehouse_activity_events AS events
		JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
		WHERE events.event = 'workspace_storage_provider.update' AND topics.topic = ?
	`, database.ActivityTopicWorkspaceStorageProvider(workspace)).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 1 {
		t.Fatalf("workspace storage provider update activity count = %d, want 1", activityCount)
	}

	state.WorkspaceStorageProviders[0].Priority = 30
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `SELECT priority, revision FROM gatehouse_workspace_storage_providers WHERE workspace = ? AND provider = (SELECT id FROM gatehouse_storage_providers WHERE alias = 'documents')`, workspace.Id).Scan(&priority, &revision); err != nil {
		t.Fatal(err)
	}
	if priority != 20 || revision != 2 {
		t.Fatalf("stale documents workspace binding = (%d, %d)", priority, revision)
	}

	state.StorageProviders[1].Alias = "documents-v2"
	state.WorkspaceStorageProviders[0].ProviderAlias = "documents-v2"
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	var renamedDocumentsID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_storage_providers WHERE alias = 'documents-v2'`).Scan(&renamedDocumentsID); err != nil {
		t.Fatal(err)
	}
	if !typed_id.Valid(typed_id.StorageProvider, renamedDocumentsID) || renamedDocumentsID == documentsID {
		t.Fatalf("renamed storage provider ID = %q, want new typed ID distinct from %q", renamedDocumentsID, documentsID)
	}
}

func TestMigrateConfiguresBuiltinAgentProviderWithoutCredentials(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{
		AgentProviders: []config.AgentProvider{{
			Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true,
		}},
		AgentModels: []config.AgentModel{{
			Alias: "fallback", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Fallback reply."}`, MaxTurns: config.DefaultAgentModelMaxTurns, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true,
		}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}

	var protocol string
	var baseURL, keychainID, apiKey sql.NullString
	var keychainVersion sql.NullInt64
	if err := store.QueryRowContext(ctx, `
		SELECT protocol, base_url, keychain_id, keychain_version, api_key
		FROM gatehouse_agent_providers
		WHERE alias = 'builtin'
	`).Scan(&protocol, &baseURL, &keychainID, &keychainVersion, &apiKey); err != nil {
		t.Fatal(err)
	}
	if protocol != "builtin" || baseURL.Valid || keychainID.Valid || keychainVersion.Valid || apiKey.Valid {
		t.Fatalf("builtin provider credentials = protocol %q base_url %#v keychain_id %#v keychain_version %#v api_key %#v", protocol, baseURL, keychainID, keychainVersion, apiKey)
	}
	var model, parameters string
	if err := store.QueryRowContext(ctx, `
		SELECT model, parameters
		FROM gatehouse_agent_models
		WHERE alias = 'fallback'
	`).Scan(&model, &parameters); err != nil {
		t.Fatal(err)
	}
	if model != "dummy.fixed-reply" || parameters != `{"text":"Fallback reply."}` {
		t.Fatalf("builtin model = (%q, %q)", model, parameters)
	}
	var providerID, modelID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_agent_providers WHERE alias = 'builtin'`).Scan(&providerID); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_agent_models WHERE alias = 'fallback'`).Scan(&modelID); err != nil {
		t.Fatal(err)
	}
	if !typed_id.Valid(typed_id.AgentProvider, providerID) || !typed_id.Valid(typed_id.AgentModel, modelID) {
		t.Fatalf("agent IDs = (%q, %q), want typed provider and model IDs", providerID, modelID)
	}

	state.AgentModels[0].Alias = "fallback-v2"
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	var renamedModelID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_agent_models WHERE alias = 'fallback-v2'`).Scan(&renamedModelID); err != nil {
		t.Fatal(err)
	}
	if !typed_id.Valid(typed_id.AgentModel, renamedModelID) || renamedModelID == modelID {
		t.Fatalf("renamed model ID = %q, want new typed ID distinct from %q", renamedModelID, modelID)
	}
}

func TestMigrateEncryptsAgentProviderAPIKeyWithAliasAAD(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	baseURL := "https://api.example.test"
	keychainID := "agent"
	state := config.State{
		Keychains: []config.Keychain{{ID: keychainID, Sources: []config.KeychainPassphraseSource{"env:AGENT_KEYCHAIN"}}},
		AgentProviders: []config.AgentProvider{{
			Alias: "openai", Revision: 1, Protocol: "openai-chat-completions", BaseURL: &baseURL, Keychain: &keychainID, Sources: []config.AgentProviderAPIKeySource{"env:AGENT_API_KEY"}, Enabled: true,
		}},
	}
	t.Setenv("AGENT_KEYCHAIN", "agent passphrase")
	t.Setenv("AGENT_API_KEY", "secret-api-key")
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}

	var apiKey string
	if err := store.QueryRowContext(ctx, `SELECT api_key FROM gatehouse_agent_providers WHERE alias = 'openai'`).Scan(&apiKey); err != nil {
		t.Fatal(err)
	}
	err, encrypted := keychain.ParseKey(apiKey)
	if err != nil {
		t.Fatal(err)
	}
	if encrypted.AAD != keychain.AADAlias {
		t.Fatalf("agent provider ciphertext AAD = %q, want %q", encrypted.AAD, keychain.AADAlias)
	}
}

func TestMigrateWithConfiguredRepeatablesAppliesStrictMigrations(t *testing.T) {
	err, database := openConfigured(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	if err := migrateState(context.Background(), database, configuration, config.State{}); err != nil {
		t.Fatalf("Migrate() after runtime repeatables: %v", err)
	}

	var gatehouseName string
	if err := database.QueryRow(`SELECT name FROM gatehouse_workspaces WHERE alias = 'gatehouse'`).Scan(&gatehouseName); err != nil {
		t.Fatal(err)
	}
	if gatehouseName != "Gatehouse" {
		t.Fatalf("Gatehouse workspace name = %q, want %q", gatehouseName, "Gatehouse")
	}

	if _, err := database.Exec(`
		INSERT INTO gatehouse_schema_migrations (
			migration_type, migration_index, description, checksum, applied_at
		) VALUES ('versioned', 'invalid', 'test', zeroblob(32), '2026-01-01T00:00:00.000Z')
	`); err == nil {
		t.Fatal("strict migration history accepted text for migration_index")
	}
}

func TestOpenEphemeralDiscardsStateAfterClose(t *testing.T) {
	context := context.Background()
	err, first := openConfigured(context, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Exec(`CREATE TABLE test_state (value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	err, second := openConfigured(context, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := second.Exec(`SELECT * FROM test_state`); err == nil {
		t.Fatal("ephemeral database retained state after close")
	}
}

func TestOpenSQLiteCreatesDatabaseFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "gatehouse.db")
	err, database := database.Open(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestOpenPostgresReportsUnsupported(t *testing.T) {
	err, _ := database.Open(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindPostgres,
		URL:  "env:GATEHOUSE_DATABASE_URL",
	})
	if err == nil || !strings.Contains(err.Error(), "not supported yet") {
		t.Fatalf("Open() error = %v, want unsupported PostgreSQL error", err)
	}
}

func TestOpenSQLiteReconcilesWorkspaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gatehouse.db")
	err, first := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, []config.Workspace{{Alias: "engineering", Name: stringPointer("Engineering"), Enabled: true}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	firstWorkspace := workspaceRef(t, context.Background(), first, "engineering")
	var activityCount int
	if err := first.QueryRow(`
		SELECT COUNT(*)
		FROM gatehouse_activity_events AS events
		JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
		WHERE events.event = 'workspace.create' AND topics.topic = ?
	`, database.ActivityTopicWorkspace(firstWorkspace)).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 1 {
		t.Fatalf("workspace create activity count = %d, want 1", activityCount)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	err, second := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, []config.Workspace{{Alias: "engineering", Name: stringPointer("Platform Engineering"), Enabled: false}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	var (
		secondID string
		name     string
		enabled  bool
	)
	secondWorkspace := workspaceRef(t, context.Background(), second, "engineering")
	if err := second.QueryRow(`
		SELECT id, name, enabled
		FROM gatehouse_workspaces
		WHERE id = ?
	`, secondWorkspace.Id).Scan(&secondID, &name, &enabled); err != nil {
		t.Fatal(err)
	}
	if secondID != firstWorkspace.Id {
		t.Fatalf("workspace id = %q, want %q", secondID, firstWorkspace.Id)
	}
	if name != "Platform Engineering" || enabled {
		t.Fatalf("workspace = (%q, %t), want (%q, %t)", name, enabled, "Platform Engineering", false)
	}
	if err := second.QueryRow(`
		SELECT COUNT(*)
		FROM gatehouse_activity_events AS events
		JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
		WHERE events.event = 'workspace.update' AND topics.topic = ?
	`, database.ActivityTopicWorkspace(secondWorkspace)).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 1 {
		t.Fatalf("workspace update activity count = %d, want 1", activityCount)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	err, third := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	thirdWorkspace := workspaceRef(t, context.Background(), third, "engineering")
	if err := third.QueryRow(`
		SELECT id, name, enabled
		FROM gatehouse_workspaces
		WHERE id = ?
	`, thirdWorkspace.Id).Scan(&secondID, &name, &enabled); err != nil {
		t.Fatal(err)
	}
	if secondID != firstWorkspace.Id || name != "Platform Engineering" || enabled {
		t.Fatalf("omitted workspace = (%q, %q, %t), want (%q, %q, %t)", secondID, name, enabled, firstWorkspace.Id, "Platform Engineering", false)
	}
}

func TestOpenSQLiteReconcilesUnnamedWorkspace(t *testing.T) {
	err, database := openConfigured(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, []config.Workspace{{
		Alias:   "engineering",
		Enabled: true,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	workspace := workspaceRef(t, context.Background(), database, "engineering")
	var name sql.NullString
	if err := database.QueryRow(`SELECT name FROM gatehouse_workspaces WHERE id = ?`, workspace.Id).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name.Valid {
		t.Fatalf("workspace name = %q, want NULL", name.String)
	}
}

func TestOpenSQLiteReconcilesPrincipalsAndIdentities(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gatehouse.db")
	principals := []config.Principal{{
		Alias:    "alice",
		Name:     stringPointer("Alice"),
		Revision: 1,
		Enabled:  true,
	}}
	err, first := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, nil, principals)
	if err != nil {
		t.Fatal(err)
	}
	alice := principalRef(t, context.Background(), first, "alice")
	identityID := newTypedID(t, typed_id.Identity)
	if err := first.IdentitiesUpsertRevisions(context.Background(), []model.Identity{{
		Id: identityID, Alias: stringPointer("alice-gatehouse"), Key: "gatehouse:alice", Principal: alice, Revision: 1, Verifiers: []interface{}{"gh-ver:first"}, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	updated := []config.Principal{{
		Alias:    "alice",
		Name:     stringPointer("Alice Example"),
		Revision: 2,
		Enabled:  false,
	}}
	err, second := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, nil, updated)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	alice = principalRef(t, context.Background(), second, "alice")
	if err := second.IdentitiesUpsertRevisions(context.Background(), []model.Identity{{
		Id: identityID, Alias: stringPointer("alice-gatehouse"), Key: "gatehouse:alice", Principal: alice, Revision: 2, Verifiers: []interface{}{"gh-ver:second"}, Enabled: false,
	}}); err != nil {
		t.Fatal(err)
	}

	var (
		principalName     string
		principalOn       bool
		principalRevision int
		identityOwner     string
		verifiers         string
		identityOn        bool
		revision          int
	)
	if err := second.QueryRow(`
		SELECT p.name, p.enabled, p.revision, i.principal_id, i.verifiers, i.enabled, i.revision
		FROM gatehouse_principals AS p
		JOIN gatehouse_identities AS i ON i.principal_id = p.id
		WHERE p.id = ? AND i.id = ?
	`, alice.Id, identityID).Scan(&principalName, &principalOn, &principalRevision, &identityOwner, &verifiers, &identityOn, &revision); err != nil {
		t.Fatal(err)
	}
	if principalName != "Alice Example" || principalOn || principalRevision != 2 || identityOwner != alice.Id || verifiers != `["gh-ver:second"]` || identityOn || revision != 2 {
		t.Fatalf("reconciled principal and identity = (%q, %t, %d, %q, %q, %t, %d)", principalName, principalOn, principalRevision, identityOwner, verifiers, identityOn, revision)
	}
	if err := migrateState(context.Background(), second, config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: path}, config.State{Principals: []config.Principal{{
		Alias: "alice", Name: stringPointer("Older"), Revision: 1, Enabled: true,
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := second.QueryRow(`SELECT name, enabled, revision FROM gatehouse_principals WHERE id = ?`, alice.Id).Scan(&principalName, &principalOn, &principalRevision); err != nil {
		t.Fatal(err)
	}
	if principalName != "Alice Example" || principalOn || principalRevision != 2 {
		t.Fatalf("older principal revision replaced principal = (%q, %t, %d)", principalName, principalOn, principalRevision)
	}
	if err := second.IdentitiesUpsertRevisions(context.Background(), []model.Identity{{
		Id: identityID, Alias: stringPointer("alice-gatehouse"), Key: "gatehouse:alice", Principal: alice, Revision: 1, Verifiers: []interface{}{"gh-ver:older"}, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := second.QueryRow(`SELECT verifiers, enabled, revision FROM gatehouse_identities WHERE id = ?`, identityID).Scan(&verifiers, &identityOn, &revision); err != nil {
		t.Fatal(err)
	}
	if verifiers != `["gh-ver:second"]` || identityOn || revision != 2 {
		t.Fatalf("lower revision replaced identity = (%q, %t, %d)", verifiers, identityOn, revision)
	}

	if _, err := second.Exec(`INSERT INTO gatehouse_identities (id, key, principal_id, verifiers, enabled) VALUES (?, 'matrix:@unknown:example.org', ?, '[{"kind":"matrix"}]', TRUE)`, newTypedID(t, typed_id.Identity), newTypedID(t, typed_id.Principal)); err == nil {
		t.Fatal("identity without a principal was accepted")
	}
	if _, err := second.Exec(`INSERT INTO gatehouse_principals (id, revision, enabled) VALUES ('alice', 1, TRUE)`); err == nil {
		t.Fatal("natural principal ID was accepted")
	}
	if _, err := second.Exec(`INSERT INTO gatehouse_identities (id, key, principal_id, verifiers, enabled) VALUES ('gatehouse:alice', 'matrix:@invalid:example.org', ?, '[{"kind":"matrix"}]', TRUE)`, alice.Id); err == nil {
		t.Fatal("natural identity ID was accepted")
	}
}

func TestOpenSQLitePreservesUnconfiguredPrincipals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gatehouse.db")
	configured := []config.Principal{{
		Alias:   "alice",
		Enabled: true,
		Identities: []config.Identity{{
			Alias:     "alice-gatehouse",
			Key:       "gatehouse:alice",
			Revision:  1,
			Verifiers: []config.Verifier{{Value: stringPointer("gh-ver:configured"), Stored: "gh-ver:configured"}},
			Enabled:   true,
		}},
	}}
	err, first := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, nil, configured)
	if err != nil {
		t.Fatal(err)
	}
	bobID := newTypedID(t, typed_id.Principal)
	bobIdentityID := newTypedID(t, typed_id.Identity)
	if _, err := first.Exec(`
		INSERT INTO gatehouse_principals (id, name, revision, enabled)
		VALUES (?, 'Bob', 1, TRUE)
	`, bobID); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Exec(`
		INSERT INTO gatehouse_identities (id, key, principal_id, verifiers, enabled)
		VALUES (?, 'matrix:@bob:example.org', ?, '[{"kind":"matrix"}]', TRUE)
	`, bobIdentityID, bobID); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	err, second := openConfigured(context.Background(), config.DatabaseConfig{
		Kind: config.DatabaseKindSQLite,
		Path: path,
	}, nil, configured)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	var count int
	if err := second.QueryRow(`
		SELECT COUNT(*)
		FROM gatehouse_identities
		WHERE id = ? AND principal_id = ?
	`, bobIdentityID, bobID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("runtime identity count = %d, want 1", count)
	}
}

func TestMigrateSQLiteReconcilesGroupsWithRuntimePrincipal(t *testing.T) {
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, database := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals: []config.Principal{{Alias: "alice", Enabled: true}},
	}
	if err := migrateState(context.Background(), database, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, context.Background(), database, "engineering")
	bobID := newTypedID(t, typed_id.Principal)
	if _, err := database.Exec(`INSERT INTO gatehouse_principals (id, alias, name, revision, enabled) VALUES (?, 'bob', 'Bob', 1, TRUE)`, bobID); err != nil {
		t.Fatal(err)
	}

	state.Groups = []config.Group{{
		WorkspaceID: "engineering",
		Alias:       "admins",
		Name:        stringPointer("Administrators"),
		Enabled:     true,
		Members: []config.GroupMember{
			{PrincipalID: "alice", Enabled: true},
			{PrincipalID: "bob", Enabled: false},
		},
	}}
	if err := migrateState(context.Background(), database, configuration, state); err != nil {
		t.Fatal(err)
	}
	adminsID := groupID(t, context.Background(), database, "engineering", "admins")

	var (
		name       string
		enabled    bool
		memberRows int
	)
	if err := database.QueryRow(`
		SELECT g.name, g.enabled, COUNT(m.principal_id)
		FROM gatehouse_groups AS g
		LEFT JOIN gatehouse_group_members AS m
			ON m.workspace_id = g.workspace_id AND m.group_id = g.id
		WHERE g.workspace_id = ? AND g.id = ?
		GROUP BY g.workspace_id, g.id
	`, workspace.Id, adminsID).Scan(&name, &enabled, &memberRows); err != nil {
		t.Fatal(err)
	}
	if name != "Administrators" || !enabled || memberRows != 2 {
		t.Fatalf("group = (%q, %t, %d members), want (%q, %t, %d members)", name, enabled, memberRows, "Administrators", true, 2)
	}

	var bobEnabled bool
	if err := database.QueryRow(`
		SELECT enabled
		FROM gatehouse_group_members
		WHERE workspace_id = ? AND group_id = ? AND principal_id = ?
	`, workspace.Id, adminsID, bobID).Scan(&bobEnabled); err != nil {
		t.Fatal(err)
	}
	if bobEnabled {
		t.Fatal("runtime principal membership is enabled, want disabled")
	}
	for _, event := range []string{"group.create", "group_member.create"} {
		var count int
		if err := database.QueryRow(`
			SELECT COUNT(*)
			FROM gatehouse_activity_events AS events
			JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
			WHERE events.event = ? AND topics.topic = ?
		`, event, workspace.Id+"/"+adminsID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		want := 1
		if event == "group_member.create" {
			want = 2
		}
		if count != want {
			t.Fatalf("%s activity count = %d, want %d", event, count, want)
		}
	}
	var aliceID string
	if err := database.QueryRow(`SELECT id FROM gatehouse_principals WHERE alias = 'alice'`).Scan(&aliceID); err != nil {
		t.Fatal(err)
	}
	var memberTopicCount int
	if err := database.QueryRow(`
		SELECT COUNT(*)
		FROM gatehouse_activity_events AS events
		JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
		WHERE events.event = 'group_member.create' AND topics.topic = ?
	`, aliceID+"/"+adminsID).Scan(&memberTopicCount); err != nil {
		t.Fatal(err)
	}
	if memberTopicCount != 1 {
		t.Fatalf("group member activity topic count = %d, want 1", memberTopicCount)
	}

	state.Groups[0].Name = stringPointer("Platform administrators")
	state.Groups[0].Members[0].Enabled = false
	if err := migrateState(context.Background(), database, configuration, state); err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"group.update", "group_member.update"} {
		var count int
		if err := database.QueryRow(`
			SELECT COUNT(*)
			FROM gatehouse_activity_events AS events
			JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
			WHERE events.event = ? AND topics.topic = ?
		`, event, workspace.Id+"/"+adminsID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s activity count = %d, want 1", event, count)
		}
	}
	if _, err := database.Exec(`
		INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled)
		VALUES (?, ?, ?, TRUE)
	`, workspace.Id, adminsID, newTypedID(t, typed_id.Principal)); err == nil {
		t.Fatal("membership without a principal was accepted")
	}
}

func TestMigrateSQLiteCoordinatesConfiguredGroupReconciliation(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindSQLite, Path: filepath.Join(t.TempDir(), "gatehouse.db")}
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals: []config.Principal{{Alias: "alice", Enabled: true}},
		Groups: []config.Group{{
			WorkspaceID: "engineering",
			Alias:       "admins",
			Enabled:     true,
			Members:     []config.GroupMember{{PrincipalID: "alice", Enabled: true}},
		}},
	}
	err, first := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	err, second := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	start := make(chan struct{})
	errors := make(chan error, 2)
	var wait sync.WaitGroup
	for _, store := range []*database.Store{first, second} {
		wait.Add(1)
		go func(store *database.Store) {
			defer wait.Done()
			<-start
			errors <- migrateState(ctx, store, configuration, state)
		}(store)
	}
	close(start)
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}

	workspace := workspaceRef(t, ctx, first, "engineering")
	for _, event := range []string{"group.create", "group_member.create"} {
		var count int
		if err := first.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM gatehouse_activity_events AS events
			JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
			WHERE events.event = ? AND topics.topic LIKE ?
		`, event, workspace.Id+"/%").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s activity count = %d, want 1", event, count)
		}
	}
}

func TestMigrateSQLiteEmitsWorkspaceAgentActivity(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	state := config.State{
		Workspaces:     []config.Workspace{{Alias: "engineering", Enabled: true}},
		AgentProviders: []config.AgentProvider{{Alias: "builtin", Revision: 1, Protocol: "builtin", Enabled: true}},
		AgentModels: []config.AgentModel{{
			Alias: "assistant", Revision: 1, ProviderAlias: "builtin", Model: "dummy.fixed-reply", Parameters: `{"text":"Hello."}`, MaxTurns: config.DefaultAgentModelMaxTurns, MaxOutputTokens: config.DefaultAgentModelMaxOutputTokens, Enabled: true,
		}},
		WorkspaceAgents: []config.WorkspaceAgent{{WorkspaceID: "engineering", ModelAlias: "assistant", Revision: 1, Priority: 1, Enabled: true}},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	var modelID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_agent_models WHERE alias = 'assistant'`).Scan(&modelID); err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"workspace_agent.create"} {
		var count int
		if err := store.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM gatehouse_activity_events AS events
			JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
			WHERE events.event = ? AND topics.topic = ?
		`, event, database.ActivityTopicWorkspaceAgent(model.WorkspaceAgentRef{Workspace: workspace, Model: model.AgentModelRef{Id: modelID}})).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s activity count = %d, want 1", event, count)
		}
	}

	state.WorkspaceAgents[0].Revision = 2
	state.WorkspaceAgents[0].Priority = 2
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM gatehouse_activity_events AS events
		JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
		WHERE events.event = 'workspace_agent.update' AND topics.topic = ?
	`, database.ActivityTopicWorkspaceAgent(model.WorkspaceAgentRef{Workspace: workspace, Model: model.AgentModelRef{Id: modelID}})).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("workspace_agent.update activity count = %d, want 1", count)
	}
	var priority, revision int
	if err := store.QueryRowContext(ctx, `SELECT priority, revision FROM gatehouse_workspace_agents WHERE workspace_id = ? AND model_id = ?`, workspace.Id, modelID).Scan(&priority, &revision); err != nil {
		t.Fatal(err)
	}
	if priority != 2 || revision != 2 {
		t.Fatalf("updated workspace agent = (%d, %d)", priority, revision)
	}

	state.WorkspaceAgents[0].Priority = 3
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `SELECT priority, revision FROM gatehouse_workspace_agents WHERE workspace_id = ? AND model_id = ?`, workspace.Id, modelID).Scan(&priority, &revision); err != nil {
		t.Fatal(err)
	}
	if priority != 2 || revision != 2 {
		t.Fatalf("stale workspace agent = (%d, %d)", priority, revision)
	}
}

func TestMigrateSQLiteReconcilesWorkspaceGrants(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	alice, members := "alice", "members"
	state := config.State{
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		Principals: []config.Principal{{Alias: alice, Enabled: true}},
		Groups:     []config.Group{{WorkspaceID: "engineering", Alias: members, Enabled: true}},
		WorkspaceGrants: []config.WorkspaceGrant{
			{WorkspaceID: "engineering", Role: "member", PrincipalID: &alice, Revision: 1, Enabled: true},
			{WorkspaceID: "engineering", Role: "manager", GroupID: &members, Revision: 1, Enabled: true},
		},
	}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	var principalID string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_principals WHERE alias = 'alice'`).Scan(&principalID); err != nil {
		t.Fatal(err)
	}
	group := groupID(t, ctx, store, "engineering", "members")
	var principalGrantID string
	if err := store.QueryRowContext(ctx, `
		SELECT id FROM gatehouse_workspace_grants
		WHERE workspace = ? AND role = 'member' AND principal = ?
	`, workspace.Id, principalID).Scan(&principalGrantID); err != nil {
		t.Fatal(err)
	}

	var grants int
	if err := store.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM gatehouse_workspace_grants
		WHERE workspace = ? AND (
			(role = 'member' AND principal = ?)
			OR (role = 'manager' AND "group" = ?)
		)
	`, workspace.Id, principalID, group).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if grants != 2 {
		t.Fatalf("workspace grant count = %d, want 2", grants)
	}
	var activityCount int
	if err := store.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM gatehouse_activity_events AS events
		JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
		WHERE events.event = 'workspace_grant.create' AND topics.topic = ?
	`, workspace.Id+"/"+principalGrantID).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 1 {
		t.Fatalf("workspace grant create activity count = %d, want 1", activityCount)
	}
	if err := store.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM gatehouse_activity_events AS events
		JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
		WHERE events.event = 'workspace_grant.create' AND topics.topic = ?
	`, principalID+"/"+principalGrantID).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 1 {
		t.Fatalf("principal workspace grant create activity count = %d, want 1", activityCount)
	}

	state.WorkspaceGrants[0].Enabled = false
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	if err := store.QueryRowContext(ctx, `
		SELECT enabled
		FROM gatehouse_workspace_grants
		WHERE workspace = ? AND role = 'member' AND principal = ?
	`, workspace.Id, principalID).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("equal revision changed the grant")
	}

	state.WorkspaceGrants[0].Revision = 2
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `
		SELECT enabled
		FROM gatehouse_workspace_grants
		WHERE workspace = ? AND role = 'member' AND principal = ?
	`, workspace.Id, principalID).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("newer revision did not change the grant")
	}
	if err := store.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM gatehouse_activity_events AS events
		JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id
		WHERE events.event = 'workspace_grant.update' AND topics.topic = ?
	`, workspace.Id+"/"+principalGrantID).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 1 {
		t.Fatalf("workspace grant update activity count = %d, want 1", activityCount)
	}
}

func TestMigrateSQLiteReconcilesSystemGrants(t *testing.T) {
	ctx := context.Background()
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{Principals: []config.Principal{{Alias: "alice", Enabled: true}}, SystemGrants: []config.SystemGrant{{PrincipalID: "alice", Revision: 1, Enabled: true}}}
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	var id, principalID string
	var enabled bool
	var revision int
	if err := store.QueryRowContext(ctx, `SELECT id, principal, enabled, revision FROM gatehouse_system_grants`).Scan(&id, &principalID, &enabled, &revision); err != nil {
		t.Fatal(err)
	}
	if !typed_id.Valid(typed_id.SystemGrant, id) || !enabled || revision != 1 {
		t.Fatalf("system grant = (%q, %t, %d), want enabled revision 1", id, enabled, revision)
	}
	err, roles := store.SystemRolesGet(ctx, model.PrincipalRef{Id: principalID})
	if err != nil || !authz.SystemAllows(roles, authz.SystemManage) {
		t.Fatalf("SystemRolesGet() = (%#v, %v), want manager", roles, err)
	}
	state.SystemGrants[0].Enabled = false
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `SELECT enabled FROM gatehouse_system_grants WHERE id = ?`, id).Scan(&enabled); err != nil || !enabled {
		t.Fatalf("equal revision changed system grant = (%t, %v)", enabled, err)
	}
	state.SystemGrants[0].Revision = 2
	if err := migrateState(ctx, store, configuration, state); err != nil {
		t.Fatal(err)
	}
	if err := store.QueryRowContext(ctx, `SELECT enabled, revision FROM gatehouse_system_grants WHERE id = ?`, id).Scan(&enabled, &revision); err != nil || enabled || revision != 2 {
		t.Fatalf("newer revision did not change system grant = (%t, %d, %v)", enabled, revision, err)
	}
	var activities int
	if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_activity_events AS events JOIN gatehouse_activity_event_topics AS topics ON topics.activity = events.id WHERE events.event = 'system_grant.update' AND topics.topic = ?`, "sys/"+id).Scan(&activities); err != nil || activities != 1 {
		t.Fatalf("system grant update activity = (%d, %v), want 1", activities, err)
	}
}

func TestMigrateSQLiteEnforcesKeychainConstraints(t *testing.T) {
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, database := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := migrateState(context.Background(), database, configuration, config.State{}); err != nil {
		t.Fatal(err)
	}

	if _, err := database.Exec(`
		INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled)
		VALUES ('default', 1, 'gh-kdf:salt?alg=pbkdf2-hmac-sha256-v1', 'gh-enc:payload?alg=aes128-gcm-v1', TRUE)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled)
		VALUES ('default', 2, 'gh-kdf:salt?alg=pbkdf2-hmac-sha256-v1', 'gh-enc:payload?alg=aes128-gcm-v1', TRUE)
	`); err != nil {
		t.Fatalf("second enabled keychain version was rejected: %v", err)
	}

	invalid := []string{
		`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('Default', 3, 'kdf', 'key', TRUE)`,
		`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('default', 0, 'kdf', 'key', TRUE)`,
		`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('default', -1, 'kdf', 'key', TRUE)`,
		`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('default', 3, ' ', 'key', TRUE)`,
		`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('default', 3, 'kdf', ' ', TRUE)`,
		`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('default', 3, 'kdf', 'key', 2)`,
		`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('default', 1, 'kdf', 'key', TRUE)`,
	}
	for _, statement := range invalid {
		if _, err := database.Exec(statement); err == nil {
			t.Fatalf("invalid keychain row was accepted: %s", statement)
		}
	}
}

func TestKeychainsInsertAndGet(t *testing.T) {
	err, database := openConfigured(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	keychains := []model.Keychain{
		{
			Ref:     model.KeychainRef{Id: "default", Version: 1},
			KekKdf:  "gh-kdf:default?alg=pbkdf2-hmac-sha256-v1",
			Key:     "gh-enc:default?alg=aes128-gcm-v1",
			Enabled: true,
		},
		{
			Ref:     model.KeychainRef{Id: "alpha", Version: 1},
			KekKdf:  "gh-kdf:alpha?alg=pbkdf2-hmac-sha256-v1",
			Key:     "gh-enc:alpha?alg=aes128-gcm-v1",
			Enabled: true,
		},
	}
	if err := database.KeychainsInsert(context.Background(), keychains); err != nil {
		t.Fatal(err)
	}
	if err := database.KeychainsInsert(context.Background(), []model.Keychain{{
		Ref:     model.KeychainRef{Id: "default", Version: 1},
		KekKdf:  "replacement",
		Key:     "replacement",
		Enabled: false,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := database.KeychainsInsert(context.Background(), nil); err != nil {
		t.Fatal(err)
	}

	err, got := database.KeychainsGet(context.Background(), []model.KeychainRef{
		{Id: "default", Version: 1},
		{Id: "missing", Version: 1},
		{Id: "alpha", Version: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []model.Keychain{keychains[1], keychains[0]}) {
		t.Fatalf("KeychainsGet() = %#v, want %#v", got, []model.Keychain{keychains[1], keychains[0]})
	}
	err, got = database.KeychainsGet(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("KeychainsGet(nil) = %#v, want no keychains", got)
	}
}

func TestKeychainsGetCurrent(t *testing.T) {
	err, database := openConfigured(context.Background(), config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	keychains := []model.Keychain{
		{Ref: model.KeychainRef{Id: "alpha", Version: 1}, KekKdf: "alpha-1", Key: "alpha-1", Enabled: true},
		{Ref: model.KeychainRef{Id: "alpha", Version: 2}, KekKdf: "alpha-2", Key: "alpha-2", Enabled: true},
		{Ref: model.KeychainRef{Id: "alpha", Version: 3}, KekKdf: "alpha-3", Key: "alpha-3", Enabled: false},
		{Ref: model.KeychainRef{Id: "default", Version: 1}, KekKdf: "default-1", Key: "default-1", Enabled: false},
	}
	if err := database.KeychainsInsert(context.Background(), keychains); err != nil {
		t.Fatal(err)
	}

	err, got := database.KeychainsGetCurrent(context.Background(), []string{"default", "missing", "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	want := []model.Keychain{keychains[1]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("KeychainsGetCurrent() = %#v, want %#v", got, want)
	}
	err, got = database.KeychainsGetCurrent(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("KeychainsGetCurrent(nil) = %#v, want no keychains", got)
	}
}

func stringPointer(value string) *string {
	return &value
}

func workspaceRef(t *testing.T, ctx context.Context, store *database.Store, alias string) model.WorkspaceRef {
	t.Helper()
	err, workspace := store.WorkspaceRefGetByAlias(ctx, alias)
	if err != nil {
		t.Fatal(err)
	}
	if workspace == nil {
		t.Fatalf("workspace alias %q was not found", alias)
	}
	return *workspace
}

func principalRef(t *testing.T, ctx context.Context, store *database.Store, alias string) model.PrincipalRef {
	t.Helper()
	var id string
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_principals WHERE alias = ?`, alias).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return model.PrincipalRef{Id: id}
}

func groupID(t *testing.T, ctx context.Context, store *database.Store, workspaceAlias, alias string) string {
	t.Helper()
	var id string
	if err := store.QueryRowContext(ctx, `
		SELECT groups.id
		FROM gatehouse_groups AS groups
		JOIN gatehouse_workspaces AS workspaces ON workspaces.id = groups.workspace_id
		WHERE workspaces.alias = ? AND groups.alias = ?
	`, workspaceAlias, alias).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func newTypedID(t *testing.T, kind string) string {
	t.Helper()
	id, err := typed_id.New(kind)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func openConfigured(ctx context.Context, configuration config.DatabaseConfig, workspaces []config.Workspace, principals []config.Principal) (error, *database.Store) {
	err, database := database.Open(ctx, configuration)
	if err != nil {
		return err, nil
	}
	if err := migrateState(ctx, database, configuration, config.State{Workspaces: workspaces, Principals: principals}); err != nil {
		database.Close()
		return err, nil
	}
	return nil, database
}

func migrateState(ctx context.Context, store *database.Store, configuration config.DatabaseConfig, state config.State) error {
	err, keyring := keychain.NewKeyring(store, state.Keychains, keychain.NewPassphraseSourceResolver())
	if err != nil {
		return err
	}
	defer keyring.Close()
	err, set := migrations.Build(configuration, state, keyring)
	if err != nil {
		return err
	}
	return migrations.Run(ctx, store, set)
}

func openMigrationTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	return database
}

func openMigrationTestFileDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	return database
}
