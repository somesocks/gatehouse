package migrations

import (
	"context"
	"encoding/json"
	"fmt"

	"gatehouse/config"
	"gatehouse/identity"
	"gatehouse/typed_id"
)

func postgresMigrationR005ReconcileIdentities(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       5,
		Description: "reconcile_identities",
		Builder:     postgresMigrationR005ReconcileIdentitiesBuilder(state.Principals),
	}
}

type postgresMigrationR005IdentityValue struct {
	ID          string
	Alias       string
	Key         string
	PrincipalID string
	Revision    int
	Verifiers   string
	Enabled     bool
}

type postgresMigrationR005Identity struct {
	ID       string
	Revision int
}

func postgresMigrationR005ReconcileIdentitiesBuilder(principals []config.Principal) MigrationBuilder {
	resolver := identity.NewPasswordSourceResolver()
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		principalIDs, err := principalIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		existing, err := postgresMigrationR005IdentitiesByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		identities := make([]postgresMigrationR005IdentityValue, 0)
		events := make([]activityMigrationEvent, 0)
		for _, principal := range principals {
			principalID := principalIDs[principal.Alias]
			if principalID == "" {
				return fmt.Errorf("principal %q was not reconciled", principal.Alias), ""
			}
			for _, configured := range principal.Identities {
				existingIdentity, exists := existing[configured.Alias]
				if exists && existingIdentity.Revision >= configured.Revision {
					continue
				}
				id := existingIdentity.ID
				if !exists {
					id, err = typed_id.New(typed_id.Identity)
					if err != nil {
						return err, ""
					}
				}
				err, verifiers := identity.ResolveVerifiers(configured.Key, configured.Verifiers, resolver)
				if err != nil {
					return fmt.Errorf("resolve verifiers for identity %q: %w", configured.Key, err), ""
				}
				encoded, err := json.Marshal(verifiers)
				if err != nil {
					return fmt.Errorf("encode verifiers for identity %q: %w", configured.Key, err), ""
				}
				identities = append(identities, postgresMigrationR005IdentityValue{
					ID: id, Alias: configured.Alias, Key: configured.Key, PrincipalID: principalID, Revision: configured.Revision, Verifiers: string(encoded), Enabled: configured.Enabled,
				})
				eventName := "identity.create"
				if exists {
					eventName = "identity.update"
				}
				event, err := newActivityMigrationEvent("", eventName, "identity", "", principalID, "", principalID+"/"+id)
				if err != nil {
					return err, ""
				}
				event.IdentityID = id
				events = append(events, event)
			}
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range .Identities }}
			INSERT INTO gatehouse_identities (id, alias, key, principal_id, verifiers, enabled, revision)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .Key }}, {{ sqlLiteral .PrincipalID }}, {{ sqlLiteral .Verifiers }}, {{ sqlBool .Enabled }}, {{ sqlLiteral .Revision }})
			ON CONFLICT (alias) DO UPDATE SET
				key = excluded.key,
				principal_id = excluded.principal_id,
				verifiers = excluded.verifiers,
				enabled = excluded.enabled,
				revision = excluded.revision
			WHERE gatehouse_identities.revision < excluded.revision;
			{{ end }}
			{{ range .Events }}
			{{ $event := . }}
			INSERT INTO gatehouse_activity_events (
				id, event, resource_kind, resource_identity, created_at
			) VALUES (
				{{ sqlLiteral .ID }}, {{ sqlLiteral .Event }}, {{ sqlLiteral .ResourceKind }}, {{ sqlLiteral .IdentityID }}, {{ sqlLiteral .CreatedAt }}
			);
			{{ range .Topics }}
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			VALUES ({{ sqlLiteral $event.ID }}, {{ sqlLiteral . }});
			{{ end }}
			{{ end }}
		`, struct {
			Identities []postgresMigrationR005IdentityValue
			Events     []activityMigrationEvent
		}{Identities: identities, Events: events})
	}
}

func postgresMigrationR005IdentitiesByAlias(ctx context.Context, session *MigrationSession) (map[string]postgresMigrationR005Identity, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, id, revision FROM gatehouse_identities WHERE alias IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("get identities by alias: %w", err)
	}
	defer rows.Close()

	identities := map[string]postgresMigrationR005Identity{}
	for rows.Next() {
		var alias string
		var identity postgresMigrationR005Identity
		if err := rows.Scan(&alias, &identity.ID, &identity.Revision); err != nil {
			return nil, fmt.Errorf("scan identity: %w", err)
		}
		identities[alias] = identity
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate identities: %w", err)
	}
	return identities, nil
}
