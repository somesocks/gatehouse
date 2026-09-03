package migrations

import (
	"context"
	"encoding/json"
	"fmt"

	"gatehouse/config"
)

func sqliteMigrationR005ReconcileIdentities(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       5,
		Description: "reconcile_identities",
		Builder:     sqliteMigrationR005ReconcileIdentitiesBuilder(state.Principals),
	}
}

type sqliteMigrationR005IdentityValue struct {
	PrincipalAlias string
	Alias          string
	Key            string
	Revision       int
	VerifierSource string
	Enabled        bool
}

func sqliteMigrationR005ReconcileIdentitiesBuilder(principals []config.Principal) MigrationBuilder {
	return func(_ context.Context, session *MigrationSession) (error, string) {
		identities := make([]sqliteMigrationR005IdentityValue, 0)
		for _, principal := range principals {
			for _, configured := range principal.Identities {
				encoded, err := json.Marshal(configured.Verifiers)
				if err != nil {
					return fmt.Errorf("encode verifier sources for identity %q: %w", configured.Key, err), ""
				}
				identities = append(identities, sqliteMigrationR005IdentityValue{
					PrincipalAlias: principal.Alias, Alias: configured.Alias, Key: configured.Key, Revision: configured.Revision, VerifierSource: string(encoded), Enabled: configured.Enabled,
				})
			}
		}
		return session.RenderTemplate(`
			DROP TABLE IF EXISTS gatehouse_migration_identity_desired;
			DROP TABLE IF EXISTS gatehouse_migration_identity_state;

			CREATE TEMP TABLE gatehouse_migration_identity_desired (
				principal_alias TEXT NOT NULL,
				alias TEXT NOT NULL PRIMARY KEY,
				key TEXT NOT NULL,
				revision INTEGER NOT NULL,
				verifier_source TEXT NOT NULL,
				enabled INTEGER NOT NULL
			) STRICT;
			{{ range . }}
			INSERT INTO gatehouse_migration_identity_desired (principal_alias, alias, key, revision, verifier_source, enabled)
			VALUES ({{ sqlLiteral .PrincipalAlias }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .Key }}, {{ sqlLiteral .Revision }}, {{ sqlLiteral .VerifierSource }}, {{ sqlBool .Enabled }});
			{{ end }}

			CREATE TEMP TABLE gatehouse_migration_identity_state AS
			SELECT desired.*, principals.id AS principal_id, identities.id AS existing_id
			FROM gatehouse_migration_identity_desired AS desired
			JOIN gatehouse_principals AS principals ON principals.alias = desired.principal_alias
			LEFT JOIN gatehouse_identities AS identities ON identities.alias = desired.alias
			WHERE identities.id IS NULL OR identities.revision < desired.revision;

			INSERT INTO gatehouse_identities (id, alias, key, principal_id, verifiers, enabled, revision)
			SELECT COALESCE(existing_id, gh_id_new('idt')), alias, key, principal_id, gh_identity_verifiers(key, verifier_source), enabled, revision
			FROM gatehouse_migration_identity_state
			WHERE TRUE
			ON CONFLICT (alias) DO UPDATE SET
				key = excluded.key,
				principal_id = excluded.principal_id,
				verifiers = excluded.verifiers,
				enabled = excluded.enabled,
				revision = excluded.revision
			WHERE gatehouse_identities.revision < excluded.revision;

			DROP TABLE gatehouse_migration_identity_state;
			DROP TABLE gatehouse_migration_identity_desired;
		`, identities)
	}
}
