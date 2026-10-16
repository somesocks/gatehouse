package migrations

import (
	"context"
	"database/sql"
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

type activityMigrationEvent struct {
	WorkspaceAlias                    string
	ID                                string
	CreatedAt                         string
	Event                             string
	ResourceKind                      string
	GroupID                           string
	PrincipalID                       string
	WorkspaceAgent                    string
	WorkspaceAgentAlias               string
	KeychainID                        string
	KeychainVersion                   int
	AgentProviderID                   string
	AgentModelID                      string
	IdentityID                        string
	StorageProviderID                 string
	WorkspaceID                       string
	WorkspaceGrantID                  string
	SystemGrantID                     string
	WorkspaceStorageProviderWorkspace string
	WorkspaceStorageProviderProvider  string
	Topics                            []string
}

func newActivityMigrationEvent(workspaceAlias, event, resourceKind, groupID, principalID, workspaceAgent string, topics ...string) (activityMigrationEvent, error) {
	id, err := typed_id.New(typed_id.ActivityEvent)
	if err != nil {
		return activityMigrationEvent{}, err
	}
	createdAt, err := typed_id.Timestamp(typed_id.ActivityEvent, id)
	if err != nil {
		return activityMigrationEvent{}, err
	}
	return activityMigrationEvent{
		WorkspaceAlias: workspaceAlias,
		ID:             id,
		CreatedAt:      createdAt.Format("2006-01-02T15:04:05.000Z"),
		Event:          event,
		ResourceKind:   resourceKind,
		GroupID:        groupID,
		PrincipalID:    principalID,
		WorkspaceAgent: workspaceAgent,
		Topics:         topics,
	}, nil
}

func sameOptionalString(value sql.NullString, expected *string) bool {
	return (expected == nil && !value.Valid) || (expected != nil && value.Valid && value.String == *expected)
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

func workspaceIDsByAlias(ctx context.Context, session *MigrationSession) (map[string]string, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, id FROM gatehouse_workspaces WHERE alias IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("get workspace IDs by alias: %w", err)
	}
	defer rows.Close()

	ids := map[string]string{}
	for rows.Next() {
		var alias, id string
		if err := rows.Scan(&alias, &id); err != nil {
			return nil, fmt.Errorf("scan workspace ID: %w", err)
		}
		ids[alias] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace IDs: %w", err)
	}
	return ids, nil
}

type groupAliasKey struct {
	Workspace string
	Alias     string
}

type storedGroup struct {
	ID       string
	Name     sql.NullString
	Enabled  bool
	Revision int
}

func groupsByAlias(ctx context.Context, session *MigrationSession) (map[groupAliasKey]storedGroup, error) {
	rows, err := session.QueryContext(ctx, `
		SELECT workspaces.alias, groups.alias, groups.id, groups.name, groups.enabled, groups.revision
		FROM gatehouse_groups AS groups
		JOIN gatehouse_workspaces AS workspaces ON workspaces.id = groups.workspace_id
		WHERE workspaces.alias IS NOT NULL AND groups.alias IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("get groups by alias: %w", err)
	}
	defer rows.Close()

	groups := map[groupAliasKey]storedGroup{}
	for rows.Next() {
		var key groupAliasKey
		var group storedGroup
		if err := rows.Scan(&key.Workspace, &key.Alias, &group.ID, &group.Name, &group.Enabled, &group.Revision); err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		groups[key] = group
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate groups: %w", err)
	}
	return groups, nil
}
