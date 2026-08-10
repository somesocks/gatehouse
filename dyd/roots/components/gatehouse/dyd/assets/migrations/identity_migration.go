package migrations

import (
	"context"
	"encoding/json"
	"fmt"

	"gatehouse/config"
	"gatehouse/identity"
)

type identityMigrationValue struct {
	ID          string
	PrincipalID string
	Revision    int
	Verifiers   string
	Enabled     bool
}

func identityMigrationBuilder(principals []config.Principal) MigrationBuilder {
	resolver := identity.NewPasswordSourceResolver()
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		err, revisions := migrationIdentityRevisions(ctx, session)
		if err != nil {
			return err, ""
		}
		identities := make([]identityMigrationValue, 0)
		for _, principal := range principals {
			for _, configured := range principal.Identities {
				if revisions[configured.ID] >= configured.Revision {
					continue
				}
				err, verifiers := identity.ResolveVerifiers(configured.ID, configured.Verifiers, resolver)
				if err != nil {
					return fmt.Errorf("resolve verifiers for identity %q: %w", configured.ID, err), ""
				}
				encoded, err := json.Marshal(verifiers)
				if err != nil {
					return fmt.Errorf("encode verifiers for identity %q: %w", configured.ID, err), ""
				}
				identities = append(identities, identityMigrationValue{
					ID: configured.ID, PrincipalID: principal.ID, Revision: configured.Revision, Verifiers: string(encoded), Enabled: configured.Enabled,
				})
			}
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_identities (id, principal_id, verifiers, enabled, revision)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .PrincipalID }}, {{ sqlLiteral .Verifiers }}, {{ sqlBool .Enabled }}, {{ sqlLiteral .Revision }})
			ON CONFLICT (id) DO UPDATE SET
				principal_id = excluded.principal_id,
				verifiers = excluded.verifiers,
				enabled = excluded.enabled,
				revision = excluded.revision
			WHERE gatehouse_identities.revision < excluded.revision;
			{{ end }}
		`, identities)
	}
}

func migrationIdentityRevisions(ctx context.Context, session *MigrationSession) (error, map[string]int) {
	rows, err := session.QueryContext(ctx, `SELECT id, revision FROM gatehouse_identities`)
	if err != nil {
		return fmt.Errorf("get identity revisions: %w", err), nil
	}
	defer rows.Close()

	revisions := make(map[string]int)
	for rows.Next() {
		var id string
		var revision int
		if err := rows.Scan(&id, &revision); err != nil {
			return fmt.Errorf("read identity revision: %w", err), nil
		}
		revisions[id] = revision
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read identity revisions: %w", err), nil
	}
	return nil, revisions
}
