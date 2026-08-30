package migrations

import (
	"context"
	"fmt"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/typed_id"
)

type Set struct {
	kind     config.DatabaseKind
	registry Registry
}

func Build(configuration config.DatabaseConfig, state config.State, keyring *keychain.Keyring) (error, Set) {
	if keyring == nil {
		return fmt.Errorf("migration keyring is required"), Set{}
	}
	switch configuration.Kind {
	case config.DatabaseKindSQLite, config.DatabaseKindEphemeral:
		err, registry := sqliteMigrations(state, keyring)
		if err != nil {
			return err, Set{}
		}
		return nil, Set{kind: config.DatabaseKindSQLite, registry: registry}
	case config.DatabaseKindPostgres:
		err, registry := postgresMigrations(state, keyring)
		if err != nil {
			return err, Set{}
		}
		return nil, Set{kind: config.DatabaseKindPostgres, registry: registry}
	default:
		return fmt.Errorf("unsupported migration database kind %q", configuration.Kind), Set{}
	}
}

func Run(ctx context.Context, store *database.Store, migrations Set) error {
	switch migrations.kind {
	case config.DatabaseKindSQLite:
		return migrateSQLite(ctx, store.DB, migrations.registry)
	case config.DatabaseKindPostgres:
		return migratePostgres(ctx, store.DB, migrations.registry)
	default:
		return fmt.Errorf("unsupported migration database kind %q", migrations.kind)
	}
}

type workspaceMigrationValue struct {
	ID      string
	Alias   string
	Name    any
	Enabled bool
}

type principalMigrationValue struct {
	ID      string
	Alias   string
	Name    any
	Enabled bool
}

type groupMigrationValue struct {
	WorkspaceID string
	ID          string
	Alias       string
	Name        any
	Enabled     bool
	Members     []groupMemberMigrationValue
}

type groupMemberMigrationValue struct {
	PrincipalID string
	Enabled     bool
}

func principalMigrationBuilder(principals []config.Principal) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		existing, err := principalIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]principalMigrationValue, 0, len(principals))
		for _, principal := range principals {
			id := existing[principal.Alias]
			if id == "" {
				id, err = typed_id.New(typed_id.Principal)
				if err != nil {
					return err, ""
				}
			}
			var name any
			if principal.Name != nil {
				name = *principal.Name
			}
			values = append(values, principalMigrationValue{ID: id, Alias: principal.Alias, Name: name, Enabled: principal.Enabled})
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_principals (id, alias, name, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .Name }}, {{ sqlBool .Enabled }})
			ON CONFLICT (alias) DO UPDATE SET
				name = excluded.name,
				enabled = excluded.enabled;
			{{ end }}
		`, values)
	}
}

func principalIDsByAlias(ctx context.Context, session *MigrationSession) (map[string]string, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, id FROM gatehouse_principals WHERE alias IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("get principal IDs by alias: %w", err)
	}
	defer rows.Close()

	ids := map[string]string{}
	for rows.Next() {
		var alias, id string
		if err := rows.Scan(&alias, &id); err != nil {
			return nil, fmt.Errorf("scan principal ID: %w", err)
		}
		ids[alias] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate principal IDs: %w", err)
	}
	return ids, nil
}

func groupMigrationBuilder(groups []config.Group) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		existing, err := groupIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]groupMigrationValue, 0, len(groups))
		for _, group := range groups {
			id := existing[groupAliasKey{Workspace: group.WorkspaceID, Alias: group.Alias}]
			if id == "" {
				id, err = typed_id.New(typed_id.Group)
				if err != nil {
					return err, ""
				}
			}
			var name any
			if group.Name != nil {
				name = *group.Name
			}
			members := make([]groupMemberMigrationValue, 0, len(group.Members))
			for _, member := range group.Members {
				members = append(members, groupMemberMigrationValue{PrincipalID: member.PrincipalID, Enabled: member.Enabled})
			}
			values = append(values, groupMigrationValue{WorkspaceID: group.WorkspaceID, ID: id, Alias: group.Alias, Name: name, Enabled: group.Enabled, Members: members})
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			{{ $group := . }}
			INSERT INTO gatehouse_groups (workspace_id, id, alias, name, enabled)
			VALUES ((SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}), {{ sqlLiteral .ID }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .Name }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, alias) DO UPDATE SET
				name = excluded.name,
				enabled = excluded.enabled;
			{{ range .Members }}
			INSERT INTO gatehouse_group_members (workspace_id, group_id, principal_id, enabled)
			VALUES ((SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral $group.WorkspaceID }}), {{ sqlLiteral $group.ID }}, (SELECT id FROM gatehouse_principals WHERE alias = {{ sqlLiteral .PrincipalID }}), {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, group_id, principal_id) DO UPDATE SET
				enabled = excluded.enabled;
			{{ end }}
			{{ end }}
		`, values)
	}
}

type groupAliasKey struct {
	Workspace string
	Alias     string
}

func groupIDsByAlias(ctx context.Context, session *MigrationSession) (map[groupAliasKey]string, error) {
	rows, err := session.QueryContext(ctx, `
		SELECT workspaces.alias, groups.alias, groups.id
		FROM gatehouse_groups AS groups
		JOIN gatehouse_workspaces AS workspaces ON workspaces.id = groups.workspace_id
		WHERE workspaces.alias IS NOT NULL AND groups.alias IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("get group IDs by alias: %w", err)
	}
	defer rows.Close()

	ids := map[groupAliasKey]string{}
	for rows.Next() {
		var key groupAliasKey
		var id string
		if err := rows.Scan(&key.Workspace, &key.Alias, &id); err != nil {
			return nil, fmt.Errorf("scan group ID: %w", err)
		}
		ids[key] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group IDs: %w", err)
	}
	return ids, nil
}
