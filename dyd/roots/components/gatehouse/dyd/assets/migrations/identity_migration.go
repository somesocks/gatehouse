package migrations

import (
	"context"
	"encoding/json"
	"fmt"

	"gatehouse/config"
	"gatehouse/identity"
	"gatehouse/typed_id"
)

type identityMigrationValue struct {
	ID          string
	Alias       string
	Key         string
	PrincipalID string
	Revision    int
	Verifiers   string
	Enabled     bool
}

func identityMigrationBuilder(principals []config.Principal) MigrationBuilder {
	resolver := identity.NewPasswordSourceResolver()
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		principalIDs, err := principalIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		existing, err := migrationIdentitiesByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		identities := make([]identityMigrationValue, 0)
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
				identities = append(identities, identityMigrationValue{
					ID: id, Alias: configured.Alias, Key: configured.Key, PrincipalID: principalID, Revision: configured.Revision, Verifiers: string(encoded), Enabled: configured.Enabled,
				})
			}
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
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
		`, identities)
	}
}

type migrationIdentity struct {
	ID       string
	Revision int
}

func migrationIdentitiesByAlias(ctx context.Context, session *MigrationSession) (map[string]migrationIdentity, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, id, revision FROM gatehouse_identities WHERE alias IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("get identities by alias: %w", err)
	}
	defer rows.Close()

	identities := map[string]migrationIdentity{}
	for rows.Next() {
		var alias string
		var identity migrationIdentity
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
