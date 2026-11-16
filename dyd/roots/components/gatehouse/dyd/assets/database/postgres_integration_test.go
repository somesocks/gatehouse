package database_test

import (
	"context"
	"net/url"
	"os"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/typed_id"

	"github.com/jackc/pgx/v5"
)

func newPostgresIntegrationTestStore(t *testing.T) *database.Store {
	t.Helper()
	if os.Getenv("GATEHOUSE_TEST_POSTGRES_URL") == "" {
		t.Skip("requires GATEHOUSE_TEST_POSTGRES_URL with disposable PostgreSQL databases and CREATE DATABASE permission")
	}
	ctx := context.Background()
	err, control := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_POSTGRES_URL"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { control.Close() })
	name := "integration_" + newTypedID(t, typed_id.Workspace)
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := control.ExecContext(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := control.ExecContext(ctx, "DROP DATABASE "+identifier); err != nil {
			t.Error(err)
		}
	})
	target, err := url.Parse(control.DatabaseURL())
	if err != nil {
		t.Fatal(err)
	}
	target.Path, target.RawPath = "/"+name, ""
	t.Setenv("GATEHOUSE_TEST_POSTGRES_DATABASE_URL", target.String())
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_POSTGRES_DATABASE_URL"}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := migrateState(ctx, store, configuration, config.State{}); err != nil {
		t.Fatal(err)
	}
	return store
}
