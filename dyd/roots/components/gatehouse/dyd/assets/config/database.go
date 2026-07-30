package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"gatehouse/configschema"
)

type DatabaseKind string

const (
	DatabaseKindSQLite    DatabaseKind = "sqlite"
	DatabaseKindPostgres  DatabaseKind = "postgres"
	DatabaseKindEphemeral DatabaseKind = "ephemeral"
)

type DatabaseConfig struct {
	Kind DatabaseKind
	Path string
	URL  string
}

type Warning struct {
	Message string
}

var environmentReference = regexp.MustCompile(`^env:[A-Za-z_][A-Za-z0-9_]*$`)

func ResolveDatabase(document configschema.GatehouseConfig) (error, DatabaseConfig, []Warning) {
	if document.Database == nil {
		err, path := defaultDatabasePath(runtime.GOOS, os.UserHomeDir, os.Getenv)
		if err != nil {
			return err, DatabaseConfig{}, nil
		}
		return nil, DatabaseConfig{Kind: DatabaseKindSQLite, Path: path}, nil
	}

	switch document.Database.Kind {
	case configschema.GatehouseConfigDatabaseKindSQLiteDatabase:
		if document.Database.SQLiteDatabase == nil {
			return fmt.Errorf("database sqlite configuration is missing"), DatabaseConfig{}, nil
		}
		path := document.Database.SQLiteDatabase.Path
		if filepath.IsAbs(path) {
			return nil, DatabaseConfig{Kind: DatabaseKindSQLite, Path: path}, nil
		}

		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("resolve relative SQLite database path: %w", err), DatabaseConfig{}, nil
		}
		return nil, DatabaseConfig{
				Kind: DatabaseKindSQLite,
				Path: absolutePath,
			}, []Warning{{
				Message: fmt.Sprintf("database.path %q is relative and resolves to %q", path, absolutePath),
			}}

	case configschema.GatehouseConfigDatabaseKindPostgresDatabase:
		if document.Database.PostgresDatabase == nil {
			return fmt.Errorf("database postgres configuration is missing"), DatabaseConfig{}, nil
		}
		url := document.Database.PostgresDatabase.Url
		if !environmentReference.MatchString(url) {
			return fmt.Errorf("database.url must be an env:VARIABLE_NAME reference"), DatabaseConfig{}, nil
		}
		return nil, DatabaseConfig{Kind: DatabaseKindPostgres, URL: url}, nil

	case configschema.GatehouseConfigDatabaseKindEphemeralDatabase:
		if document.Database.EphemeralDatabase == nil {
			return fmt.Errorf("database ephemeral configuration is missing"), DatabaseConfig{}, nil
		}
		return nil, DatabaseConfig{Kind: DatabaseKindEphemeral}, nil

	default:
		return fmt.Errorf("unsupported database kind %q", document.Database.Kind), DatabaseConfig{}, nil
	}
}

func defaultDatabasePath(goos string, userHomeDir func() (string, error), getenv func(string) string) (error, string) {
	var dataDir string
	switch goos {
	case "darwin":
		home, err := userHomeDir()
		if err != nil {
			return fmt.Errorf("find user home directory: %w", err), ""
		}
		dataDir = filepath.Join(home, "Library", "Application Support")
	case "windows":
		dataDir = getenv("LOCALAPPDATA")
		if dataDir == "" {
			home, err := userHomeDir()
			if err != nil {
				return fmt.Errorf("find user home directory: %w", err), ""
			}
			dataDir = filepath.Join(home, "AppData", "Local")
		}
	default:
		dataDir = getenv("XDG_DATA_HOME")
		if dataDir == "" {
			home, err := userHomeDir()
			if err != nil {
				return fmt.Errorf("find user home directory: %w", err), ""
			}
			dataDir = filepath.Join(home, ".local", "share")
		}
	}

	return nil, filepath.Join(dataDir, "gatehouse", "gatehouse.db")
}
