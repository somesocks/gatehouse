package migrations

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"gatehouse/config"
	"gatehouse/configschema"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/typed_id"
)

func TestSQLiteDefaultAdmissionFullRegistry(t *testing.T) {
	for _, scenario := range []string{"fresh", "grant revoked", "membership revoked", "downgraded"} {
		t.Run(scenario, func(t *testing.T) {
			configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
			err, store := database.Open(context.Background(), configuration)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			testDefaultAdmissionFullRegistry(t, configuration, store, scenario)
		})
	}
}

func TestPostgresDefaultAdmissionFullRegistry(t *testing.T) {
	if os.Getenv("GATEHOUSE_TEST_ADMISSION_BOOTSTRAP_POSTGRES_URL") == "" {
		t.Skip("requires disposable PostgreSQL server with CREATE DATABASE permission")
	}
	ctx := context.Background()
	err, control := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_ADMISSION_BOOTSTRAP_POSTGRES_URL"})
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	for _, scenario := range []string{"fresh", "grant revoked", "membership revoked", "downgraded"} {
		t.Run(scenario, func(t *testing.T) {
			id, err := typed_id.New(typed_id.Workspace)
			if err != nil {
				t.Fatal(err)
			}
			name := "bootstrap_" + strings.TrimPrefix(id, "wsp_")
			err, identifier := sqlIdentifier(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := control.ExecContext(ctx, "CREATE DATABASE "+identifier); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := control.ExecContext(ctx, "DROP DATABASE "+identifier); err != nil {
					t.Error(err)
				}
			})
			target, err := url.Parse(os.Getenv("GATEHOUSE_TEST_ADMISSION_BOOTSTRAP_POSTGRES_URL"))
			if err != nil {
				t.Fatal(err)
			}
			target.Path, target.RawPath = "/"+name, ""
			t.Setenv("GATEHOUSE_TEST_ADMISSION_DATABASE_POSTGRES_URL", target.String())
			configuration := config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_ADMISSION_DATABASE_POSTGRES_URL"}
			err, store := database.Open(ctx, configuration)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { store.Close() })
			testDefaultAdmissionFullRegistry(t, configuration, store, scenario)
		})
	}
}

func testDefaultAdmissionFullRegistry(t *testing.T, configuration config.DatabaseConfig, store *database.Store, scenario string) {
	t.Helper()
	t.Setenv("GATEHOUSE_KEYCHAIN", "bootstrap-regression-passphrase")
	t.Setenv("GATEHOUSE_ROOT_PASSWORD", "bootstrap-regression-password")
	ctx := context.Background()
	err, state := config.ResolveState(configschema.GatehouseConfig{ApiVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	registry := func() Registry {
		t.Helper()
		err, ring := keychain.NewKeyring(store, state.Keychains, keychain.NewPassphraseSourceResolver())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(ring.Close)
		var result Registry
		if configuration.Kind == config.DatabaseKindPostgres {
			err, result = postgresMigrations(state, ring)
		} else {
			err, result = sqliteMigrations(state, ring)
		}
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	migrate := func(registry Registry) error {
		if configuration.Kind == config.DatabaseKindPostgres {
			return migratePostgres(ctx, store.DB, registry)
		}
		return migrateSQLite(ctx, store.DB, registry)
	}
	if scenario != "fresh" {
		old := testSchemaAtVersion(registry(), 62)
		if err := migrate(old); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ExecContext(ctx, `
			INSERT INTO gatehouse_workspaces (id, alias, enabled) VALUES ('wsp_00000000000000000000000000', 'admin', TRUE);
			INSERT INTO gatehouse_principals (id, alias, revision, enabled) VALUES ('prn_00000000000000000000000000', 'root', 1, TRUE);
			INSERT INTO gatehouse_groups (workspace_id, id, alias, revision, enabled) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'renamed-root', 3, TRUE);
			INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled, revision) VALUES ('wsp_00000000000000000000000000', 'grp_00000000000000000000000000', 'prn_00000000000000000000000000', TRUE, 1);
			INSERT INTO gatehouse_workspace_grants (id, workspace, role, "group", enabled, revision) VALUES ('wgr_00000000000000000000000000', 'wsp_00000000000000000000000000', 'manager', 'grp_00000000000000000000000000', TRUE, 1);
		`); err != nil {
			t.Fatal(err)
		}
		change := map[string]string{
			"grant revoked":      `UPDATE gatehouse_workspace_grants SET enabled = FALSE, revision = 3`,
			"membership revoked": `UPDATE gatehouse_group_members SET enabled = FALSE, revision = 3`,
			"downgraded":         `UPDATE gatehouse_workspace_grants SET role = 'contributor', revision = 3`,
		}[scenario]
		if _, err := store.ExecContext(ctx, change); err != nil {
			t.Fatal(err)
		}
	}
	for restart := range 3 {
		if err := migrate(registry()); err != nil {
			t.Fatalf("full registry restart %d: %v", restart, err)
		}
		rows, err := store.QueryContext(ctx, `SELECT grants.role FROM gatehouse_workspace_grants grants JOIN gatehouse_principals principal ON principal.id = grants.principal JOIN gatehouse_workspaces workspace ON workspace.id = grants.workspace WHERE principal.alias = 'root' AND workspace.alias = 'admin' AND grants.enabled = TRUE ORDER BY grants.role`)
		if err != nil {
			t.Fatal(err)
		}
		roles := []string{}
		for rows.Next() {
			var role string
			if err := rows.Scan(&role); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			roles = append(roles, role)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		want := ""
		if scenario == "fresh" {
			want = "admin"
		}
		if scenario == "downgraded" {
			want = "member"
		}
		if strings.Join(roles, ",") != want {
			t.Fatalf("restart %d changed root authority: %#v, want %q", restart, roles, want)
		}
		var groups, implicit int
		if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_groups`).Scan(&groups); err != nil {
			t.Fatal(err)
		}
		if err := store.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_groups WHERE alias = 'root'`).Scan(&implicit); err != nil {
			t.Fatal(err)
		}
		wantGroups := 1
		if scenario == "fresh" {
			wantGroups = 0
		}
		if groups != wantGroups || implicit != 0 {
			t.Fatalf("restart %d recreated a bootstrap group: groups=%d implicit=%d", restart, groups, implicit)
		}
		if scenario != "fresh" {
			var enabled bool
			var revision int
			if err := store.QueryRowContext(ctx, `SELECT enabled, revision FROM gatehouse_group_members WHERE principal_id = 'prn_00000000000000000000000000'`).Scan(&enabled, &revision); err != nil {
				t.Fatal(err)
			}
			wantEnabled, wantRevision := true, 1
			if scenario == "membership revoked" {
				wantEnabled, wantRevision = false, 3
			}
			if enabled != wantEnabled || revision != wantRevision {
				t.Fatalf("restart %d rewrote retained membership: enabled=%t revision=%d", restart, enabled, revision)
			}
		}
	}
}
