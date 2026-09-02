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

type activityMigrationEvent struct {
	WorkspaceAlias string
	ID             string
	CreatedAt      string
	Event          string
	ResourceKind   string
	GroupID        string
	PrincipalID    string
	WorkspaceAgent string
	Topics         []string
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
		existing, err := groupsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		principals, err := principalIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		membersByGroup, err := groupMembersByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]groupMigrationValue, 0, len(groups))
		events := []activityMigrationEvent{}
		for _, group := range groups {
			key := groupAliasKey{Workspace: group.WorkspaceID, Alias: group.Alias}
			stored, exists := existing[key]
			id := stored.ID
			if id == "" {
				id, err = typed_id.New(typed_id.Group)
				if err != nil {
					return err, ""
				}
			}
			if !exists {
				event, err := newActivityMigrationEvent(group.WorkspaceID, "group.create", "group", id, "", "", "group/"+id)
				if err != nil {
					return err, ""
				}
				events = append(events, event)
			} else if stored.Enabled != group.Enabled || !sameOptionalString(stored.Name, group.Name) {
				event, err := newActivityMigrationEvent(group.WorkspaceID, "group.update", "group", id, "", "", "group/"+id)
				if err != nil {
					return err, ""
				}
				events = append(events, event)
			}
			var name any
			if group.Name != nil {
				name = *group.Name
			}
			memberValues := make([]groupMemberMigrationValue, 0, len(group.Members))
			for _, member := range group.Members {
				memberValues = append(memberValues, groupMemberMigrationValue{PrincipalID: member.PrincipalID, Enabled: member.Enabled})
				principalID := principals[member.PrincipalID]
				storedMember, memberExists := membersByGroup[key][member.PrincipalID]
				if !memberExists || storedMember != member.Enabled {
					eventName := "group_member.create"
					if memberExists {
						eventName = "group_member.update"
					}
					event, err := newActivityMigrationEvent(group.WorkspaceID, eventName, "group_member", id, principalID, "", "group_member/"+id+"-"+principalID, "group/"+id)
					if err != nil {
						return err, ""
					}
					events = append(events, event)
				}
			}
			values = append(values, groupMigrationValue{WorkspaceID: group.WorkspaceID, ID: id, Alias: group.Alias, Name: name, Enabled: group.Enabled, Members: memberValues})
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range .Groups }}
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
			{{ range .Events }}
			{{ $event := . }}
			INSERT INTO gatehouse_activity_events (
				workspace, id, event, resource_kind, project, session, session_event, "group", principal, workspace_agent, created_at
			) VALUES (
				(SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceAlias }}), {{ sqlLiteral .ID }}, {{ sqlLiteral .Event }}, {{ sqlLiteral .ResourceKind }}, NULL, NULL, NULL, {{ sqlLiteral .GroupID }}, {{ if eq .ResourceKind "group_member" }}{{ sqlLiteral .PrincipalID }}{{ else }}NULL{{ end }}, NULL, {{ sqlLiteral .CreatedAt }}
			);
			{{ range .Topics }}
			INSERT INTO gatehouse_activity_event_topics (workspace, activity, topic, created_at)
			VALUES ((SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral $event.WorkspaceAlias }}), {{ sqlLiteral $event.ID }}, {{ sqlLiteral . }}, {{ sqlLiteral $event.CreatedAt }});
			{{ end }}
			{{ end }}
		`, struct {
			Groups []groupMigrationValue
			Events []activityMigrationEvent
		}{Groups: values, Events: events})
	}
}

type groupAliasKey struct {
	Workspace string
	Alias     string
}

type storedGroup struct {
	ID      string
	Name    sql.NullString
	Enabled bool
}

func groupsByAlias(ctx context.Context, session *MigrationSession) (map[groupAliasKey]storedGroup, error) {
	rows, err := session.QueryContext(ctx, `
		SELECT workspaces.alias, groups.alias, groups.id, groups.name, groups.enabled
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
		if err := rows.Scan(&key.Workspace, &key.Alias, &group.ID, &group.Name, &group.Enabled); err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}
		groups[key] = group
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate groups: %w", err)
	}
	return groups, nil
}

func groupMembersByAlias(ctx context.Context, session *MigrationSession) (map[groupAliasKey]map[string]bool, error) {
	rows, err := session.QueryContext(ctx, `
		SELECT workspaces.alias, groups.alias, principals.alias, members.enabled
		FROM gatehouse_group_members AS members
		JOIN gatehouse_groups AS groups ON groups.workspace_id = members.workspace_id AND groups.id = members.group_id
		JOIN gatehouse_workspaces AS workspaces ON workspaces.id = groups.workspace_id
		JOIN gatehouse_principals AS principals ON principals.id = members.principal_id
		WHERE workspaces.alias IS NOT NULL AND groups.alias IS NOT NULL AND principals.alias IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("get group members by alias: %w", err)
	}
	defer rows.Close()

	members := map[groupAliasKey]map[string]bool{}
	for rows.Next() {
		var key groupAliasKey
		var principal string
		var enabled bool
		if err := rows.Scan(&key.Workspace, &key.Alias, &principal, &enabled); err != nil {
			return nil, fmt.Errorf("scan group member: %w", err)
		}
		if members[key] == nil {
			members[key] = map[string]bool{}
		}
		members[key][principal] = enabled
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group members: %w", err)
	}
	return members, nil
}
