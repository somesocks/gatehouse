package migrations

import (
	"context"
	"fmt"

	"gatehouse/config"
	"gatehouse/typed_id"
)

func postgresMigrationR007ReconcileWorkspaceGrants(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       7,
		Description: "reconcile_workspace_grants",
		Builder:     postgresMigrationR007ReconcileWorkspaceGrantsBuilder(state.WorkspaceGrants),
	}
}

type postgresMigrationR007WorkspaceGrantValue struct {
	ID          string
	WorkspaceID string
	Role        string
	SubjectID   string
	Revision    int
	Enabled     bool
}

type postgresMigrationR007WorkspaceGrantValues struct {
	Principals []postgresMigrationR007WorkspaceGrantValue
	Groups     []postgresMigrationR007WorkspaceGrantValue
}

func postgresMigrationR007ReconcileWorkspaceGrantsBuilder(grants []config.WorkspaceGrant) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		existing, err := postgresMigrationR007WorkspaceGrantIDs(ctx, session)
		if err != nil {
			return err, ""
		}
		workspaceIDs, err := workspaceIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		principalIDs, err := principalIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		groups, err := groupsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := postgresMigrationR007WorkspaceGrantValues{
			Principals: make([]postgresMigrationR007WorkspaceGrantValue, 0, len(grants)),
			Groups:     make([]postgresMigrationR007WorkspaceGrantValue, 0, len(grants)),
		}
		events := make([]activityMigrationEvent, 0, len(grants))
		for _, grant := range grants {
			value := postgresMigrationR007WorkspaceGrantValue{WorkspaceID: grant.WorkspaceID, Role: grant.Role, Revision: grant.Revision, Enabled: grant.Enabled}
			kind := "principal"
			if grant.PrincipalID != nil {
				value.SubjectID = *grant.PrincipalID
			} else {
				kind = "group"
				value.SubjectID = *grant.GroupID
			}
			key := postgresMigrationR007WorkspaceGrantKey{WorkspaceID: value.WorkspaceID, Role: value.Role, SubjectKind: kind, SubjectID: value.SubjectID}
			stored, exists := existing[key]
			value.ID = stored.ID
			if !exists {
				value.ID, err = typed_id.New(typed_id.WorkspaceGrant)
				if err != nil {
					return err, ""
				}
			}
			if !exists || stored.Revision < value.Revision {
				workspaceID := workspaceIDs[value.WorkspaceID]
				if workspaceID == "" {
					return fmt.Errorf("workspace %q is unavailable", value.WorkspaceID), ""
				}
				topics := []string{workspaceID + "/" + value.ID}
				if kind == "principal" {
					principalID := principalIDs[value.SubjectID]
					if principalID == "" {
						return fmt.Errorf("principal %q is unavailable", value.SubjectID), ""
					}
					topics = append(topics, principalID+"/"+value.ID)
				} else {
					group := groups[groupAliasKey{Workspace: value.WorkspaceID, Alias: value.SubjectID}]
					if group.ID == "" {
						return fmt.Errorf("group %q is unavailable in workspace %q", value.SubjectID, value.WorkspaceID), ""
					}
					topics[0] = workspaceID + "/" + group.ID + "/" + value.ID
				}
				eventName := "workspace_grant.create"
				if exists {
					eventName = "workspace_grant.update"
				}
				event, err := newActivityMigrationEvent(value.WorkspaceID, eventName, "workspace_grant", "", "", "", topics...)
				if err != nil {
					return err, ""
				}
				event.WorkspaceGrantID = value.ID
				events = append(events, event)
			}
			if kind == "principal" {
				values.Principals = append(values.Principals, value)
			} else {
				values.Groups = append(values.Groups, value)
			}
		}
		return session.RenderTemplate(`
		SELECT 1;
		{{ range .Principals }}
		INSERT INTO gatehouse_workspace_grants (id, workspace, role, principal, "group", enabled, revision)
		VALUES (
			{{ sqlLiteral .ID }},
			(SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}), {{ sqlLiteral .Role }},
			(SELECT id FROM gatehouse_principals WHERE alias = {{ sqlLiteral .SubjectID }}), NULL,
			{{ sqlBool .Enabled }}, {{ sqlLiteral .Revision }}
		)
		ON CONFLICT (workspace, role, principal) WHERE principal IS NOT NULL DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
		WHERE gatehouse_workspace_grants.revision < excluded.revision;
		{{ end }}
		{{ range .Groups }}
		INSERT INTO gatehouse_workspace_grants (id, workspace, role, principal, "group", enabled, revision)
		VALUES (
			{{ sqlLiteral .ID }},
			(SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}), {{ sqlLiteral .Role }}, NULL,
			(SELECT id FROM gatehouse_groups WHERE workspace_id = (SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}) AND alias = {{ sqlLiteral .SubjectID }}),
			{{ sqlBool .Enabled }}, {{ sqlLiteral .Revision }}
		)
		ON CONFLICT (workspace, role, "group") WHERE "group" IS NOT NULL DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
		WHERE gatehouse_workspace_grants.revision < excluded.revision;
		{{ end }}
		{{ range .Events }}
		{{ $event := . }}
		INSERT INTO gatehouse_activity_events (
			id, event, resource_kind, resource_workspace_grant, created_at
		) VALUES (
			{{ sqlLiteral .ID }}, {{ sqlLiteral .Event }}, {{ sqlLiteral .ResourceKind }}, {{ sqlLiteral .WorkspaceGrantID }}, {{ sqlLiteral .CreatedAt }}
		);
		{{ range .Topics }}
		INSERT INTO gatehouse_activity_event_topics (activity, topic)
		VALUES ({{ sqlLiteral $event.ID }}, {{ sqlLiteral . }});
		{{ end }}
		{{ end }}
		`, struct {
			Principals []postgresMigrationR007WorkspaceGrantValue
			Groups     []postgresMigrationR007WorkspaceGrantValue
			Events     []activityMigrationEvent
		}{Principals: values.Principals, Groups: values.Groups, Events: events})
	}
}

type postgresMigrationR007WorkspaceGrantKey struct {
	WorkspaceID string
	Role        string
	SubjectKind string
	SubjectID   string
}

type postgresMigrationR007WorkspaceGrant struct {
	ID       string
	Revision int
}

func postgresMigrationR007WorkspaceGrantIDs(ctx context.Context, session *MigrationSession) (map[postgresMigrationR007WorkspaceGrantKey]postgresMigrationR007WorkspaceGrant, error) {
	rows, err := session.QueryContext(ctx, `
		SELECT workspaces.alias, grants.role, 'principal', principals.alias, grants.id, grants.revision
		FROM gatehouse_workspace_grants AS grants
		JOIN gatehouse_workspaces AS workspaces ON workspaces.id = grants.workspace
		JOIN gatehouse_principals AS principals ON principals.id = grants.principal
		WHERE grants.principal IS NOT NULL
		UNION ALL
		SELECT workspaces.alias, grants.role, 'group', groups.alias, grants.id, grants.revision
		FROM gatehouse_workspace_grants AS grants
		JOIN gatehouse_workspaces AS workspaces ON workspaces.id = grants.workspace
		JOIN gatehouse_groups AS groups ON groups.workspace_id = grants.workspace AND groups.id = grants."group"
		WHERE grants."group" IS NOT NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grants := map[postgresMigrationR007WorkspaceGrantKey]postgresMigrationR007WorkspaceGrant{}
	for rows.Next() {
		var key postgresMigrationR007WorkspaceGrantKey
		var grant postgresMigrationR007WorkspaceGrant
		if err := rows.Scan(&key.WorkspaceID, &key.Role, &key.SubjectKind, &key.SubjectID, &grant.ID, &grant.Revision); err != nil {
			return nil, err
		}
		grants[key] = grant
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return grants, nil
}
