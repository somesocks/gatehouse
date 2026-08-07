package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"gatehouse/config"
	_ "modernc.org/sqlite"
)

func Open(ctx context.Context, configuration config.DatabaseConfig) (error, *sql.DB) {
	switch configuration.Kind {
	case config.DatabaseKindSQLite:
		if err := prepareSQLitePath(configuration.Path); err != nil {
			return err, nil
		}
		return openSQLite(ctx, configuration.Path)
	case config.DatabaseKindEphemeral:
		return openSQLite(ctx, ":memory:")
	case config.DatabaseKindPostgres:
		return fmt.Errorf("PostgreSQL databases are not supported yet"), nil
	default:
		return fmt.Errorf("unsupported database kind %q", configuration.Kind), nil
	}
}

type MigrationSet struct {
	kind     config.DatabaseKind
	registry Registry
}

func BuildMigrations(configuration config.DatabaseConfig, state config.State) (error, MigrationSet) {
	switch configuration.Kind {
	case config.DatabaseKindSQLite, config.DatabaseKindEphemeral:
		err, registry := sqliteMigrations(state)
		if err != nil {
			return err, MigrationSet{}
		}
		return nil, MigrationSet{kind: config.DatabaseKindSQLite, registry: registry}
	case config.DatabaseKindPostgres:
		err, registry := postgresMigrations(state)
		if err != nil {
			return err, MigrationSet{}
		}
		return nil, MigrationSet{kind: config.DatabaseKindPostgres, registry: registry}
	default:
		return fmt.Errorf("unsupported database kind %q", configuration.Kind), MigrationSet{}
	}
}

func Migrate(ctx context.Context, database *sql.DB, migrations MigrationSet) error {
	switch migrations.kind {
	case config.DatabaseKindSQLite:
		return migrateSQLite(ctx, database, migrations.registry)
	case config.DatabaseKindPostgres:
		return migratePostgres(ctx, database, migrations.registry)
	default:
		return fmt.Errorf("unsupported migration database kind %q", migrations.kind)
	}
}

type migrationValues struct {
	Workspaces []workspaceMigrationValue
	Principals []principalMigrationValue
	Groups     []groupMigrationValue
}

type workspaceMigrationValue struct {
	ID      string
	Name    any
	Enabled bool
}

type principalMigrationValue struct {
	ID         string
	Name       any
	Enabled    bool
	Identities []identityMigrationValue
}

type identityMigrationValue struct {
	ID        string
	Verifiers string
	Enabled   bool
}

type groupMigrationValue struct {
	WorkspaceID string
	ID          string
	Name        any
	Enabled     bool
	Members     []groupMemberMigrationValue
}

type groupMemberMigrationValue struct {
	PrincipalID string
	Enabled     bool
}

func migrationValuesFor(state config.State) migrationValues {
	values := migrationValues{
		Workspaces: make([]workspaceMigrationValue, 0, len(state.Workspaces)),
		Principals: make([]principalMigrationValue, 0, len(state.Principals)),
		Groups:     make([]groupMigrationValue, 0, len(state.Groups)),
	}
	for _, workspace := range state.Workspaces {
		var name any
		if workspace.Name != nil {
			name = *workspace.Name
		}
		values.Workspaces = append(values.Workspaces, workspaceMigrationValue{
			ID:      workspace.ID,
			Name:    name,
			Enabled: workspace.Enabled,
		})
	}
	for _, principal := range state.Principals {
		var name any
		if principal.Name != nil {
			name = *principal.Name
		}
		identities := make([]identityMigrationValue, 0, len(principal.Identities))
		for _, identity := range principal.Identities {
			identities = append(identities, identityMigrationValue{
				ID:        identity.ID,
				Verifiers: identity.Verifiers,
				Enabled:   identity.Enabled,
			})
		}
		values.Principals = append(values.Principals, principalMigrationValue{
			ID:         principal.ID,
			Name:       name,
			Enabled:    principal.Enabled,
			Identities: identities,
		})
	}
	for _, group := range state.Groups {
		var name any
		if group.Name != nil {
			name = *group.Name
		}
		members := make([]groupMemberMigrationValue, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, groupMemberMigrationValue{
				PrincipalID: member.PrincipalID,
				Enabled:     member.Enabled,
			})
		}
		values.Groups = append(values.Groups, groupMigrationValue{
			WorkspaceID: group.WorkspaceID,
			ID:          group.ID,
			Name:        name,
			Enabled:     group.Enabled,
			Members:     members,
		})
	}
	return values
}

func prepareSQLitePath(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create SQLite database directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("create SQLite database file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close SQLite database file: %w", err)
	}
	return nil
}

func openSQLite(ctx context.Context, source string) (error, *sql.DB) {
	database, err := sql.Open("sqlite", source)
	if err != nil {
		return fmt.Errorf("open SQLite database: %w", err), nil
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return fmt.Errorf("ping SQLite database: %w", err), nil
	}
	if _, err := database.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		database.Close()
		return fmt.Errorf("enable SQLite foreign keys: %w", err), nil
	}
	return nil, database
}
