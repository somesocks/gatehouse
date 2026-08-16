package migrations

import (
	"context"
	"fmt"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
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
	Principals []principalMigrationValue
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
	Name    any
	Enabled bool
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
	Source      string
	Enabled     bool
}

type resourceMigrationValue struct {
	WorkspaceID string
	ID          string
	Source      string
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
		values.Workspaces = append(values.Workspaces, workspaceMigrationValue{Alias: workspace.Alias, Name: name, Enabled: workspace.Enabled})
	}
	for _, principal := range state.Principals {
		var name any
		if principal.Name != nil {
			name = *principal.Name
		}
		values.Principals = append(values.Principals, principalMigrationValue{ID: principal.ID, Name: name, Enabled: principal.Enabled})
	}
	for _, tool := range state.Tools {
		values.Tools = append(values.Tools, toolMigrationValue{WorkspaceID: tool.WorkspaceID, ID: tool.ID, Source: tool.Source, Enabled: tool.Enabled})
	}
	for _, resource := range state.Resources {
		values.Resources = append(values.Resources, resourceMigrationValue{WorkspaceID: resource.WorkspaceID, ID: resource.ID, Source: resource.Source, Secret: resource.Secret, Enabled: resource.Enabled})
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
			toolGrants = append(toolGrants, groupToolGrantMigrationValue{ToolID: grant.ToolID, Enabled: grant.Enabled})
		}
		resourceGrants := make([]groupResourceGrantMigrationValue, 0, len(group.ResourceGrants))
		for _, grant := range group.ResourceGrants {
			resourceGrants = append(resourceGrants, groupResourceGrantMigrationValue{ResourceID: grant.ResourceID, Enabled: grant.Enabled})
		}
		values.Groups = append(values.Groups, groupMigrationValue{
			WorkspaceID: group.WorkspaceID, ID: group.ID, Name: name, Enabled: group.Enabled,
			Members: members, ToolGrants: toolGrants, ResourceGrants: resourceGrants,
		})
	}
	return values
}
