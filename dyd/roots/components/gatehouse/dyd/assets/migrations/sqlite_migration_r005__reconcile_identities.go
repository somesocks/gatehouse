package migrations

import (
	"context"
	"encoding/json"
	"fmt"

	"gatehouse/config"
	"gatehouse/identity"
)

func sqliteMigrationR005ReconcileIdentities(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       5,
		Description: "reconcile_identities",
		Builder:     sqliteMigrationR005ReconcileIdentitiesBuilder(state.Principals),
	}
}

type sqliteMigrationR005IdentityValue struct {
	Alias       string
	Key         string
	PrincipalID string
	Revision    int
	Verifiers   string
	Enabled     bool
}

func sqliteMigrationR005ReconcileIdentitiesBuilder(principals []config.Principal) MigrationBuilder {
	resolver := identity.NewPasswordSourceResolver()
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		principalIDs, err := principalIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		existing, err := sqliteMigrationR005IdentityRevisionsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		identities := make([]sqliteMigrationR005IdentityValue, 0)
		for _, principal := range principals {
			principalID := principalIDs[principal.Alias]
			if principalID == "" {
				return fmt.Errorf("principal %q was not reconciled", principal.Alias), ""
			}
			for _, configured := range principal.Identities {
				if existing[configured.Alias] >= configured.Revision {
					continue
				}
				err, verifiers := identity.ResolveVerifiers(configured.Key, configured.Verifiers, resolver)
				if err != nil {
					return fmt.Errorf("resolve verifiers for identity %q: %w", configured.Key, err), ""
				}
				encoded, err := json.Marshal(verifiers)
				if err != nil {
					return fmt.Errorf("encode verifiers for identity %q: %w", configured.Key, err), ""
				}
				identities = append(identities, sqliteMigrationR005IdentityValue{
					Alias: configured.Alias, Key: configured.Key, PrincipalID: principalID, Revision: configured.Revision, Verifiers: string(encoded), Enabled: configured.Enabled,
				})
			}
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_identities (id, alias, key, principal_id, verifiers, enabled, revision)
			VALUES (COALESCE((SELECT id FROM gatehouse_identities WHERE alias = {{ sqlLiteral .Alias }}), gh_id_new('idt')), {{ sqlLiteral .Alias }}, {{ sqlLiteral .Key }}, {{ sqlLiteral .PrincipalID }}, {{ sqlLiteral .Verifiers }}, {{ sqlBool .Enabled }}, {{ sqlLiteral .Revision }})
			ON CONFLICT (alias) DO UPDATE SET
				key = excluded.key,
				principal_id = excluded.principal_id,
				verifiers = excluded.verifiers,
				enabled = excluded.enabled,
				revision = excluded.revision
			WHERE gatehouse_identities.revision < excluded.revision;
			{{ end }}
		`, identities)
	}
}

func sqliteMigrationR005IdentityRevisionsByAlias(ctx context.Context, session *MigrationSession) (map[string]int, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, revision FROM gatehouse_identities WHERE alias IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("get identity revisions: %w", err)
	}
	defer rows.Close()

	revisions := map[string]int{}
	for rows.Next() {
		var alias string
		var revision int
		if err := rows.Scan(&alias, &revision); err != nil {
			return nil, fmt.Errorf("scan identity revision: %w", err)
		}
		revisions[alias] = revision
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate identity revisions: %w", err)
	}
	return revisions, nil
}
