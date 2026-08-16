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

type migrationValues struct {
	Workspaces []workspaceMigrationValue
	Tools      []toolMigrationValue
	Resources  []resourceMigrationValue
	Groups     []groupMigrationValue
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
	WorkspaceID    string
	ID             string
	Alias          string
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
	Alias       string
	Source      string
	Enabled     bool
}

type resourceMigrationValue struct {
	WorkspaceID string
	ID          string
	Alias       string
	Source      string
	Secret      bool
	Enabled     bool
}

type groupToolGrantMigrationValue struct {
	ToolAlias string
	Enabled   bool
}

type groupResourceGrantMigrationValue struct {
	ResourceAlias string
	Enabled       bool
}

func migrationValuesFor(state config.State) migrationValues {
	values := migrationValues{
		Workspaces: make([]workspaceMigrationValue, 0, len(state.Workspaces)),
		Tools:      make([]toolMigrationValue, 0, len(state.Tools)),
		Resources:  make([]resourceMigrationValue, 0, len(state.Resources)),
		Groups:     make([]groupMigrationValue, 0, len(state.Groups)),
	}
	for _, workspace := range state.Workspaces {
		var name any
		if workspace.Name != nil {
			name = *workspace.Name
		}
		values.Workspaces = append(values.Workspaces, workspaceMigrationValue{Alias: workspace.Alias, Name: name, Enabled: workspace.Enabled})
	}
	for _, tool := range state.Tools {
		values.Tools = append(values.Tools, toolMigrationValue{WorkspaceID: tool.WorkspaceID, Alias: tool.Alias, Source: tool.Source, Enabled: tool.Enabled})
	}
	for _, resource := range state.Resources {
		values.Resources = append(values.Resources, resourceMigrationValue{WorkspaceID: resource.WorkspaceID, Alias: resource.Alias, Source: resource.Source, Secret: resource.Secret, Enabled: resource.Enabled})
	}
	for _, group := range state.Groups {
		var name any
		if group.Name != nil {
			name = *group.Name
		}
		members := make([]groupMemberMigrationValue, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, groupMemberMigrationValue{PrincipalID: member.PrincipalID, Enabled: member.Enabled})
		}
		toolGrants := make([]groupToolGrantMigrationValue, 0, len(group.ToolGrants))
		for _, grant := range group.ToolGrants {
			toolGrants = append(toolGrants, groupToolGrantMigrationValue{ToolAlias: grant.ToolAlias, Enabled: grant.Enabled})
		}
		resourceGrants := make([]groupResourceGrantMigrationValue, 0, len(group.ResourceGrants))
		for _, grant := range group.ResourceGrants {
			resourceGrants = append(resourceGrants, groupResourceGrantMigrationValue{ResourceAlias: grant.ResourceAlias, Enabled: grant.Enabled})
		}
		values.Groups = append(values.Groups, groupMigrationValue{
			WorkspaceID: group.WorkspaceID, Alias: group.Alias, Name: name, Enabled: group.Enabled,
			Members: members, ToolGrants: toolGrants, ResourceGrants: resourceGrants,
		})
	}
	return values
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

func toolsResourcesMigrationBuilder(tools []config.Tool, resources []config.Resource) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		toolIDs, err := toolIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		resourceIDs, err := resourceIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := migrationValues{Tools: make([]toolMigrationValue, 0, len(tools)), Resources: make([]resourceMigrationValue, 0, len(resources))}
		for _, tool := range tools {
			key := workspaceAliasKey{Workspace: tool.WorkspaceID, Alias: tool.Alias}
			id := toolIDs[key]
			if id == "" {
				id, err = typed_id.New(typed_id.Tool)
				if err != nil {
					return err, ""
				}
			}
			values.Tools = append(values.Tools, toolMigrationValue{WorkspaceID: tool.WorkspaceID, ID: id, Alias: tool.Alias, Source: tool.Source, Enabled: tool.Enabled})
		}
		for _, resource := range resources {
			key := workspaceAliasKey{Workspace: resource.WorkspaceID, Alias: resource.Alias}
			id := resourceIDs[key]
			if id == "" {
				id, err = typed_id.New(typed_id.Resource)
				if err != nil {
					return err, ""
				}
			}
			values.Resources = append(values.Resources, resourceMigrationValue{WorkspaceID: resource.WorkspaceID, ID: id, Alias: resource.Alias, Source: resource.Source, Secret: resource.Secret, Enabled: resource.Enabled})
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range .Tools }}
			INSERT INTO gatehouse_tools (workspace_id, id, alias, source, enabled)
			VALUES ((SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}), {{ sqlLiteral .ID }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .Source }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, alias) DO UPDATE SET
				source = excluded.source,
				enabled = excluded.enabled;
			{{ end }}
			{{ range .Resources }}
			INSERT INTO gatehouse_resources (workspace_id, id, alias, source, secret, enabled)
			VALUES ((SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}), {{ sqlLiteral .ID }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .Source }}, {{ sqlBool .Secret }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace_id, alias) DO UPDATE SET
				source = excluded.source,
				secret = excluded.secret,
				enabled = excluded.enabled;
			{{ end }}
		`, values)
	}
}

type workspaceAliasKey struct {
	Workspace string
	Alias     string
}

func toolIDsByAlias(ctx context.Context, session *MigrationSession) (map[workspaceAliasKey]string, error) {
	return idsByAlias(ctx, session, "gatehouse_tools", "tool")
}

func resourceIDsByAlias(ctx context.Context, session *MigrationSession) (map[workspaceAliasKey]string, error) {
	return idsByAlias(ctx, session, "gatehouse_resources", "resource")
}

func idsByAlias(ctx context.Context, session *MigrationSession, table, kind string) (map[workspaceAliasKey]string, error) {
	rows, err := session.QueryContext(ctx, `
		SELECT workspaces.alias, entities.alias, entities.id
		FROM `+table+` AS entities
		JOIN gatehouse_workspaces AS workspaces ON workspaces.id = entities.workspace_id
		WHERE workspaces.alias IS NOT NULL AND entities.alias IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("get %s IDs by alias: %w", kind, err)
	}
	defer rows.Close()

	ids := map[workspaceAliasKey]string{}
	for rows.Next() {
		var key workspaceAliasKey
		var id string
		if err := rows.Scan(&key.Workspace, &key.Alias, &id); err != nil {
			return nil, fmt.Errorf("scan %s ID: %w", kind, err)
		}
		ids[key] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s IDs: %w", kind, err)
	}
	return ids, nil
}
