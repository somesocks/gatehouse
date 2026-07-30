package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gatehouse/configschema"
)

func TestResolveDatabaseUsesDefaultSQLitePath(t *testing.T) {
	err, result, warnings := ResolveDatabase(configschema.GatehouseConfig{ApiVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != DatabaseKindSQLite || result.Path == "" {
		t.Fatalf("ResolveDatabase() = %#v, want default SQLite path", result)
	}
	if len(warnings) != 0 {
		t.Fatalf("ResolveDatabase() warnings = %#v, want none", warnings)
	}
}

func TestResolveDatabaseWarnsForRelativeSQLitePath(t *testing.T) {
	document := configschema.GatehouseConfig{
		ApiVersion: "v1",
		Database: &configschema.GatehouseConfigDatabase{
			Kind: configschema.GatehouseConfigDatabaseKindSQLiteDatabase,
			SQLiteDatabase: &configschema.GatehouseConfigDatabaseOption0{
				Kind: "sqlite",
				Path: "data/gatehouse.db",
			},
		},
	}

	err, result, warnings := ResolveDatabase(document)
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != filepath.Join(mustWorkingDirectory(t), "data", "gatehouse.db") {
		t.Fatalf("ResolveDatabase() path = %q", result.Path)
	}
	if len(warnings) != 1 {
		t.Fatalf("ResolveDatabase() warnings = %#v, want one warning", warnings)
	}
}

func TestResolveDatabaseRejectsInvalidPostgresReference(t *testing.T) {
	document := configschema.GatehouseConfig{
		ApiVersion: "v1",
		Database: &configschema.GatehouseConfigDatabase{
			Kind: configschema.GatehouseConfigDatabaseKindPostgresDatabase,
			PostgresDatabase: &configschema.GatehouseConfigDatabaseOption1{
				Kind: "postgres",
				Url:  "postgresql://gatehouse@localhost/gatehouse",
			},
		},
	}

	if err, _, _ := ResolveDatabase(document); err == nil {
		t.Fatal("ResolveDatabase() succeeded for a plaintext PostgreSQL URL")
	}
}

func TestDefaultDatabasePath(t *testing.T) {
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{
			name: "Linux XDG data directory",
			goos: "linux",
			env:  map[string]string{"XDG_DATA_HOME": "/data"},
			want: filepath.Join("/data", "gatehouse", "gatehouse.db"),
		},
		{
			name: "Linux fallback",
			goos: "linux",
			want: filepath.Join("/home/gatehouse", ".local", "share", "gatehouse", "gatehouse.db"),
		},
		{
			name: "macOS",
			goos: "darwin",
			want: filepath.Join("/home/gatehouse", "Library", "Application Support", "gatehouse", "gatehouse.db"),
		},
		{
			name: "Windows local application data",
			goos: "windows",
			env:  map[string]string{"LOCALAPPDATA": "/local-app-data"},
			want: filepath.Join("/local-app-data", "gatehouse", "gatehouse.db"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err, got := defaultDatabasePath(test.goos, func() (string, error) {
				return "/home/gatehouse", nil
			}, func(key string) string {
				return test.env[key]
			})
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("defaultDatabasePath() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDefaultDatabasePathUsesXDGWithoutHome(t *testing.T) {
	err, got := defaultDatabasePath("linux", func() (string, error) {
		return "", errors.New("HOME is not set")
	}, func(key string) string {
		if key == "XDG_DATA_HOME" {
			return "/data"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/data", "gatehouse", "gatehouse.db")
	if got != want {
		t.Fatalf("defaultDatabasePath() = %q, want %q", got, want)
	}
}

func mustWorkingDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return directory
}
