package migrations

import (
	"context"

	"gatehouse/config"
	"gatehouse/typed_id"
)

func postgresMigrationR004ReconcilePrincipals(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       4,
		Description: "reconcile_principals",
		Builder:     postgresMigrationR004ReconcilePrincipalsBuilder(state.Principals),
	}
}

type postgresMigrationR004PrincipalValue struct {
	ID       string
	Alias    string
	Name     any
	Revision int
	Enabled  bool
}

func postgresMigrationR004ReconcilePrincipalsBuilder(principals []config.Principal) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		existing, err := postgresMigrationR004PrincipalsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]postgresMigrationR004PrincipalValue, 0, len(principals))
		events := make([]activityMigrationEvent, 0, len(principals))
		for _, principal := range principals {
			if principal.Revision == 0 {
				principal.Revision = config.DefaultPrincipalRevision
			}
			previous, exists := existing[principal.Alias]
			if exists && previous.Revision >= principal.Revision {
				continue
			}
			id := previous.ID
			if !exists {
				id, err = typed_id.New(typed_id.Principal)
				if err != nil {
					return err, ""
				}
			}
			var name any
			if principal.Name != nil {
				name = *principal.Name
			}
			values = append(values, postgresMigrationR004PrincipalValue{ID: id, Alias: principal.Alias, Name: name, Revision: principal.Revision, Enabled: principal.Enabled})
			eventName := "principal.create"
			if exists {
				eventName = "principal.update"
			}
			event, err := newActivityMigrationEvent("", eventName, "principal", "", id, "", id)
			if err != nil {
				return err, ""
			}
			events = append(events, event)
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range .Values }}
			INSERT INTO gatehouse_principals (id, alias, name, revision, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .Name }}, {{ sqlLiteral .Revision }}, {{ sqlBool .Enabled }})
			ON CONFLICT (alias) DO UPDATE SET name = excluded.name, revision = excluded.revision, enabled = excluded.enabled
			WHERE gatehouse_principals.revision < excluded.revision;
			{{ end }}
			{{ range .Events }}
			{{ $event := . }}
			INSERT INTO gatehouse_activity_events (
				id, event, resource_kind, resource_principal, created_at
			) VALUES (
				{{ sqlLiteral .ID }}, {{ sqlLiteral .Event }}, {{ sqlLiteral .ResourceKind }}, {{ sqlLiteral .PrincipalID }}, {{ sqlLiteral .CreatedAt }}
			);
			{{ range .Topics }}
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			VALUES ({{ sqlLiteral $event.ID }}, {{ sqlLiteral . }});
			{{ end }}
			{{ end }}
		`, struct {
			Values []postgresMigrationR004PrincipalValue
			Events []activityMigrationEvent
		}{Values: values, Events: events})
	}
}

type postgresMigrationR004Principal struct {
	ID       string
	Revision int
}

func postgresMigrationR004PrincipalsByAlias(ctx context.Context, session *MigrationSession) (map[string]postgresMigrationR004Principal, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, id, revision FROM gatehouse_principals WHERE alias IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	principals := map[string]postgresMigrationR004Principal{}
	for rows.Next() {
		var alias string
		var principal postgresMigrationR004Principal
		if err := rows.Scan(&alias, &principal.ID, &principal.Revision); err != nil {
			return nil, err
		}
		principals[alias] = principal
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return principals, nil
}
