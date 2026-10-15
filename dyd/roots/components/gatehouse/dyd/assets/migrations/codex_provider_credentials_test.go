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

func TestSQLiteCodexCredentialMigration(t *testing.T) {
	for _, baseline := range []bool{false, true} {
		name := "versioned"
		if baseline {
			name = "baseline"
		}
		t.Run(name, func(t *testing.T) {
			db := openConfiguredMigrationTestDatabase(t)
			registry := testSQLiteRegistry(t)
			if baseline {
				registry = testSQLiteBaselineRegistry(t)
			}
			registry.Repeatable = nil
			verifyCodexMigration(t, db, registry, func(registry Registry) error { return migrateSQLite(context.Background(), db, registry) })
			rows, err := db.Query(`PRAGMA foreign_key_check`)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			if rows.Next() {
				t.Fatal("provider rebuild broke foreign keys")
			}
		})
	}
}

func TestPostgresCodexCredentialMigration(t *testing.T) {
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
	verifyCodexMigration(t, store.DB, registry, func(registry Registry) error { return migratePostgres(ctx, store.DB, registry) })
}

func verifyCodexMigration(t *testing.T, db *sql.DB, registry Registry, migrate func(Registry) error) {
	t.Helper()
	old := registry
	old.Versioned = nil
	for _, migration := range registry.Versioned {
		if migration.Index < 56 {
			old.Versioned = append(old.Versioned, migration)
		}
	}
	if err := migrate(old); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`INSERT INTO gatehouse_keychains(id,version,kek_kdf,key,enabled) VALUES('codex_migration',1,'kdf','key',TRUE)`,
		`INSERT INTO gatehouse_agent_providers(id,alias,revision,protocol,endpoint,keychain_id,keychain_version,api_key,enabled) VALUES('apr_0000000000000000000000000a','legacy',7,'openai-responses','https://api.example.test/responses','codex_migration',1,'unchanged-ciphertext',FALSE)`,
		`INSERT INTO gatehouse_agent_models(id,alias,revision,provider_id,model,parameters,enabled) VALUES('amd_0000000000000000000000000a','legacy',1,'apr_0000000000000000000000000a','test','{}',TRUE)`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	for iteration := 0; iteration < 2; iteration++ {
		if err := migrate(registry); err != nil {
			t.Fatal(err)
		}
	}
	var ciphertext, providerID string
	var revision int
	var enabled bool
	if err := db.QueryRow(`SELECT credential,revision,enabled FROM gatehouse_agent_providers WHERE alias='legacy'`).Scan(&ciphertext, &revision, &enabled); err != nil {
		t.Fatal(err)
	}
	if ciphertext != "unchanged-ciphertext" || revision != 7 || enabled {
		t.Fatal("legacy provider changed")
	}
	if err := db.QueryRow(`SELECT provider_id FROM gatehouse_agent_models WHERE alias='legacy'`).Scan(&providerID); err != nil || providerID != "apr_0000000000000000000000000a" {
		t.Fatal("model reference changed")
	}
	if _, err := db.Exec(`INSERT INTO gatehouse_agent_providers(id,alias,revision,protocol,endpoint,keychain_id,keychain_version,enabled) VALUES('apr_0000000000000000000000000b','codex',1,'openai-codex','https://chatgpt.com/backend-api/codex/responses','codex_migration',1,TRUE)`); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`UPDATE gatehouse_agent_providers SET keychain_id=NULL WHERE alias='codex'`,
		`UPDATE gatehouse_agent_providers SET endpoint='https://other.test' WHERE alias='codex'`,
		`UPDATE gatehouse_agent_providers SET credential_metadata='{}' WHERE alias='codex'`,
		`UPDATE gatehouse_agent_providers SET credential=NULL WHERE alias='legacy'`,
	} {
		if _, err := db.Exec(query); err == nil {
			t.Fatalf("invalid provider shape accepted: %s", query)
		}
	}
}
