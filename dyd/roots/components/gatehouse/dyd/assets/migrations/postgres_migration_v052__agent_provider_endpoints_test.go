package migrations

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
)

// Supply a disposable PostgreSQL database to exercise real upgrade SQL and
// migration history.
func TestPostgresMigrationV052AgentProviderEndpoints(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_MIGRATION_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL database")
	}
	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_MIGRATION_POSTGRES_URL"})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	err, ring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer ring.Close()
	err, registry := postgresMigrations(config.State{}, ring)
	if err != nil {
		t.Fatal(err)
	}
	registry.Repeatable = nil
	old := registry
	old.Versioned = nil
	for _, migration := range registry.Versioned {
		if migration.Index < 52 {
			old.Versioned = append(old.Versioned, migration)
		}
	}
	db := store.DB
	if err := migratePostgres(ctx, db, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO gatehouse_keychains (id, version, kek_kdf, key, enabled) VALUES ('migration', 1, 'kdf', 'encrypted-key', TRUE)`); err != nil {
		t.Fatal(err)
	}
	providers := []struct{ id, protocol, base, want string }{
		{"apr_00000000000000000000000001", "openai-responses", "https://proxy.invalid/v1///?api-version=1#part", "https://proxy.invalid/v1/responses?api-version=1#part"},
		{"apr_00000000000000000000000002", "openai-chat-completions", "https://proxy.invalid/prefix?route=%2F#part", "https://proxy.invalid/prefix/chat/completions?route=%2F#part"},
		{"apr_00000000000000000000000003", "openai-responses", "https://proxy.invalid", "https://proxy.invalid/responses"},
	}
	for _, provider := range providers {
		if _, err := db.Exec(`INSERT INTO gatehouse_agent_providers (id, alias, revision, protocol, base_url, keychain_id, keychain_version, api_key, enabled)
			VALUES ($1, $2, 7, $3, $4, 'migration', 1, 'encrypted-api-key', FALSE)`, provider.id, provider.id, provider.protocol, provider.base); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO gatehouse_agent_providers (id, alias, revision, protocol, enabled) VALUES ('apr_00000000000000000000000004', 'builtin', 3, 'builtin', TRUE)`); err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 2; iteration++ {
		if err := migratePostgres(ctx, db, registry); err != nil {
			t.Fatal(err)
		}
		rows, err := db.Query(`SELECT endpoint, revision, api_key, enabled FROM gatehouse_agent_providers WHERE protocol <> 'builtin' ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		index := 0
		for rows.Next() {
			var endpoint, key string
			var revision int
			var enabled bool
			if err := rows.Scan(&endpoint, &revision, &key, &enabled); err != nil {
				t.Fatal(err)
			}
			if index >= len(providers) || endpoint != providers[index].want || revision != 7 || key != "encrypted-api-key" || enabled {
				t.Fatalf("upgraded row: %q revision=%d key=%q enabled=%v", endpoint, revision, key, enabled)
			}
			index++
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if index != len(providers) {
			t.Fatalf("got %d provider rows", index)
		}
		var builtinEndpoint sql.NullString
		if err := db.QueryRow(`SELECT endpoint FROM gatehouse_agent_providers WHERE protocol = 'builtin'`).Scan(&builtinEndpoint); err != nil || builtinEndpoint.Valid {
			t.Fatalf("builtin endpoint = %v (%v)", builtinEndpoint, err)
		}
	}
}
