package migrations

import (
	"context"
	"fmt"

	"gatehouse/config"
	"gatehouse/typed_id"
)

func postgresMigrationR014ReconcileSystemGrants(state config.State) RepeatableMigration {
	return RepeatableMigration{Index: 14, Description: "reconcile_system_grants", Builder: postgresMigrationR014ReconcileSystemGrantsBuilder(state.SystemGrants)}
}

type postgresMigrationR014SystemGrantValue struct {
	ID, PrincipalID string
	Revision        int
	Enabled         bool
}

func postgresMigrationR014ReconcileSystemGrantsBuilder(grants []config.SystemGrant) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		principalIDs, err := principalIDsByAlias(ctx, session)
		if err != nil { return err, "" }
		rows, err := session.QueryContext(ctx, `SELECT principals.alias, grants.id, grants.revision FROM gatehouse_system_grants AS grants JOIN gatehouse_principals AS principals ON principals.id = grants.principal`)
		if err != nil { return err, "" }
		existing := map[string]struct{ ID string; Revision int }{}
		for rows.Next() {
			var alias string
			var grant struct{ ID string; Revision int }
			if err := rows.Scan(&alias, &grant.ID, &grant.Revision); err != nil { rows.Close(); return err, "" }
			existing[alias] = grant
		}
		if err := rows.Err(); err != nil { rows.Close(); return err, "" }
		rows.Close()
		values := []postgresMigrationR014SystemGrantValue{}
		events := []activityMigrationEvent{}
		for _, grant := range grants {
			principalID := principalIDs[grant.PrincipalID]
			if principalID == "" { return fmt.Errorf("principal %q is unavailable", grant.PrincipalID), "" }
			stored, exists := existing[grant.PrincipalID]
			if exists && stored.Revision >= grant.Revision { continue }
			id := stored.ID
			if !exists {
				id, err = typed_id.New(typed_id.SystemGrant)
				if err != nil { return err, "" }
			}
			eventName := "system_grant.create"
			if exists { eventName = "system_grant.update" }
			event, err := newActivityMigrationEvent("", eventName, "system_grant", "", principalID, "", "sys/"+id, principalID+"/"+id)
			if err != nil { return err, "" }
			event.SystemGrantID = id
			events = append(events, event)
			values = append(values, postgresMigrationR014SystemGrantValue{ID: id, PrincipalID: grant.PrincipalID, Revision: grant.Revision, Enabled: grant.Enabled})
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range .Values }}
			INSERT INTO gatehouse_system_grants (id, principal, role, enabled, revision)
			VALUES ({{ sqlLiteral .ID }}, (SELECT id FROM gatehouse_principals WHERE alias = {{ sqlLiteral .PrincipalID }}), 'manager', {{ sqlBool .Enabled }}, {{ sqlLiteral .Revision }})
			ON CONFLICT (principal) DO UPDATE SET enabled = excluded.enabled, revision = excluded.revision
			WHERE gatehouse_system_grants.revision < excluded.revision;
			{{ end }}
			{{ range .Events }}{{ $event := . }}
			INSERT INTO gatehouse_activity_events (id, event, resource_kind, resource_system_grant, created_at)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Event }}, 'system_grant', {{ sqlLiteral .SystemGrantID }}, {{ sqlLiteral .CreatedAt }});
			{{ range .Topics }}INSERT INTO gatehouse_activity_event_topics (activity, topic) VALUES ({{ sqlLiteral $event.ID }}, {{ sqlLiteral . }});{{ end }}
			{{ end }}
		`, struct { Values []postgresMigrationR014SystemGrantValue; Events []activityMigrationEvent }{values, events})
	}
}
