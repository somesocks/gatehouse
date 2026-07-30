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

func ResolveDatabase(document configschema.GatehouseConfig) (DatabaseConfig, []Warning, error) {
	if document.Database == nil {
		path, err := defaultDatabasePath(runtime.GOOS, os.UserHomeDir, os.Getenv)
		if err != nil {
			return DatabaseConfig{}, nil, err
		}
		return DatabaseConfig{Kind: DatabaseKindSQLite, Path: path}, nil, nil
	}

	switch document.Database.Kind {
	case configschema.GatehouseConfigDatabaseKindSQLiteDatabase:
		if document.Database.SQLiteDatabase == nil {
			return DatabaseConfig{}, nil, fmt.Errorf("database sqlite configuration is missing")
		}
		path := document.Database.SQLiteDatabase.Path
		if filepath.IsAbs(path) {
			return DatabaseConfig{Kind: DatabaseKindSQLite, Path: path}, nil, nil
		}

		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return DatabaseConfig{}, nil, fmt.Errorf("resolve relative SQLite database path: %w", err)
		}
		return DatabaseConfig{
				Kind: DatabaseKindSQLite,
				Path: absolutePath,
			}, []Warning{{
				Message: fmt.Sprintf("database.path %q is relative and resolves to %q", path, absolutePath),
			}}, nil

	case configschema.GatehouseConfigDatabaseKindPostgresDatabase:
		if document.Database.PostgresDatabase == nil {
			return DatabaseConfig{}, nil, fmt.Errorf("database postgres configuration is missing")
		}
		url := document.Database.PostgresDatabase.Url
		if !environmentReference.MatchString(url) {
			return DatabaseConfig{}, nil, fmt.Errorf("database.url must be an env:VARIABLE_NAME reference")
		}
		return DatabaseConfig{Kind: DatabaseKindPostgres, URL: url}, nil, nil

	case configschema.GatehouseConfigDatabaseKindEphemeralDatabase:
		if document.Database.EphemeralDatabase == nil {
			return DatabaseConfig{}, nil, fmt.Errorf("database ephemeral configuration is missing")
		}
		return DatabaseConfig{Kind: DatabaseKindEphemeral}, nil, nil

	default:
		return DatabaseConfig{}, nil, fmt.Errorf("unsupported database kind %q", document.Database.Kind)
	}
}

func defaultDatabasePath(goos string, userHomeDir func() (string, error), getenv func(string) string) (string, error) {
	var dataDir string
	switch goos {
	case "darwin":
		home, err := userHomeDir()
		if err != nil {
			return "", fmt.Errorf("find user home directory: %w", err)
		}
		dataDir = filepath.Join(home, "Library", "Application Support")
	case "windows":
		dataDir = getenv("LOCALAPPDATA")
		if dataDir == "" {
			home, err := userHomeDir()
			if err != nil {
				return "", fmt.Errorf("find user home directory: %w", err)
			}
			dataDir = filepath.Join(home, "AppData", "Local")
		}
	default:
		dataDir = getenv("XDG_DATA_HOME")
		if dataDir == "" {
			home, err := userHomeDir()
			if err != nil {
				return "", fmt.Errorf("find user home directory: %w", err)
			}
			dataDir = filepath.Join(home, ".local", "share")
		}
	}

	return filepath.Join(dataDir, "gatehouse", "gatehouse.db"), nil
}
