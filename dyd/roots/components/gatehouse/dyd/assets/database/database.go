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

type Store struct {
	*sql.DB
	kind config.DatabaseKind
}

func Open(ctx context.Context, configuration config.DatabaseConfig) (error, *Store) {
	switch configuration.Kind {
	case config.DatabaseKindSQLite:
		if err := prepareSQLitePath(configuration.Path); err != nil {
			return err, nil
		}
		err, database := openSQLite(ctx, configuration.Path)
		if err != nil {
			return err, nil
		}
		return nil, &Store{DB: database, kind: config.DatabaseKindSQLite}
	case config.DatabaseKindEphemeral:
		err, database := openSQLite(ctx, ":memory:")
		if err != nil {
			return err, nil
		}
		return nil, &Store{DB: database, kind: config.DatabaseKindSQLite}
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

func Migrate(ctx context.Context, database *Store, migrations MigrationSet) error {
	switch migrations.kind {
	case config.DatabaseKindSQLite:
		return migrateSQLite(ctx, database.DB, migrations.registry)
	case config.DatabaseKindPostgres:
		return migratePostgres(ctx, database.DB, migrations.registry)
	default:
		return fmt.Errorf("unsupported migration database kind %q", migrations.kind)
	}
}

type migrationValues struct {
	Workspaces []workspaceMigrationValue
	Principals []principalMigrationValue
	Tools      []toolMigrationValue
	Resources  []resourceMigrationValue
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
	WorkspaceID    string
	ID             string
	Name           any
	Enabled        bool
	Members        []groupMemberMigrationValue
	ToolGrants     []groupToolGrantMigrationValue
	ResourceGrants []groupResourceGrantMigrationValue
}

type groupMemberMigrationValue struct {
	PrincipalID string
	Enabled     bool
}

type toolMigrationValue struct {
	WorkspaceID string
	ID          string
	Ref         string
	Enabled     bool
}

type resourceMigrationValue struct {
	WorkspaceID string
	ID          string
	Ref         string
	Secret      bool
	Enabled     bool
}

type groupToolGrantMigrationValue struct {
	ToolID  string
	Enabled bool
}

type groupResourceGrantMigrationValue struct {
	ResourceID string
	Enabled    bool
}

func migrationValuesFor(state config.State) migrationValues {
	values := migrationValues{
		Workspaces: make([]workspaceMigrationValue, 0, len(state.Workspaces)),
		Principals: make([]principalMigrationValue, 0, len(state.Principals)),
		Tools:      make([]toolMigrationValue, 0, len(state.Tools)),
		Resources:  make([]resourceMigrationValue, 0, len(state.Resources)),
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
	for _, tool := range state.Tools {
		values.Tools = append(values.Tools, toolMigrationValue{
			WorkspaceID: tool.WorkspaceID,
			ID:          tool.ID,
			Ref:         tool.Ref,
			Enabled:     tool.Enabled,
		})
	}
	for _, resource := range state.Resources {
		values.Resources = append(values.Resources, resourceMigrationValue{
			WorkspaceID: resource.WorkspaceID,
			ID:          resource.ID,
			Ref:         resource.Ref,
			Secret:      resource.Secret,
			Enabled:     resource.Enabled,
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
		toolGrants := make([]groupToolGrantMigrationValue, 0, len(group.ToolGrants))
		for _, grant := range group.ToolGrants {
			toolGrants = append(toolGrants, groupToolGrantMigrationValue{
				ToolID:  grant.ToolID,
				Enabled: grant.Enabled,
			})
		}
		resourceGrants := make([]groupResourceGrantMigrationValue, 0, len(group.ResourceGrants))
		for _, grant := range group.ResourceGrants {
			resourceGrants = append(resourceGrants, groupResourceGrantMigrationValue{
				ResourceID: grant.ResourceID,
				Enabled:    grant.Enabled,
			})
		}
		values.Groups = append(values.Groups, groupMigrationValue{
			WorkspaceID:    group.WorkspaceID,
			ID:             group.ID,
			Name:           name,
			Enabled:        group.Enabled,
			Members:        members,
			ToolGrants:     toolGrants,
			ResourceGrants: resourceGrants,
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
	if _, err := database.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", sqliteBusyTimeout)); err != nil {
		database.Close()
		return fmt.Errorf("set SQLite busy timeout: %w", err), nil
	}
	if _, err := database.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		database.Close()
		return fmt.Errorf("enable SQLite foreign keys: %w", err), nil
	}
	return nil, database
}
