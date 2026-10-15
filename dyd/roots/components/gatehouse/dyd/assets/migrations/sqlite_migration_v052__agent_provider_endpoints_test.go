package migrations

import (
	"context"
	"database/sql"
	"testing"
)

func TestSQLiteMigrationV052AgentProviderEndpoints(t *testing.T) {
	for _, baseline := range []bool{false, true} {
		name := "versioned"
		if baseline {
			name = "baseline"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			db := openConfiguredMigrationTestDatabase(t)
			registry := testSQLiteRegistry(t)
			if baseline {
				registry = testSQLiteBaselineRegistry(t)
			}
			registry.Repeatable = nil
			old := registry
			old.Versioned = nil
			for _, migration := range registry.Versioned {
				if migration.Index < 52 {
					old.Versioned = append(old.Versioned, migration)
				}
			}
			if err := migrateSQLite(ctx, db, old); err != nil {
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
					VALUES (?, ?, 7, ?, ?, 'migration', 1, 'encrypted-api-key', FALSE)`, provider.id, provider.id, provider.protocol, provider.base); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec(`INSERT INTO gatehouse_agent_providers (id, alias, revision, protocol, enabled) VALUES ('apr_00000000000000000000000004', 'builtin', 3, 'builtin', TRUE)`); err != nil {
				t.Fatal(err)
			}
			for iteration := 0; iteration < 2; iteration++ {
				if err := migrateSQLite(ctx, db, registry); err != nil {
					t.Fatal(err)
				}
				rows, err := db.Query(`SELECT endpoint, revision, credential, enabled FROM gatehouse_agent_providers WHERE protocol <> 'builtin' ORDER BY id`)
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
		})
	}
}
