package migrations

import (
	"context"

	"gatehouse/config"
)

func sqliteMigrationR004ReconcilePrincipals(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       4,
		Description: "reconcile_principals",
		Builder:     sqliteMigrationR004ReconcilePrincipalsBuilder(state.Principals),
	}
}

func sqliteMigrationR004ReconcilePrincipalsBuilder(principals []config.Principal) MigrationBuilder {
	for index := range principals {
		if principals[index].Revision == 0 {
			principals[index].Revision = config.DefaultPrincipalRevision
		}
	}
	return func(_ context.Context, session *MigrationSession) (error, string) {
		return session.RenderTemplate(`
			DROP TABLE IF EXISTS gatehouse_migration_principal_desired;

			CREATE TEMP TABLE gatehouse_migration_principal_desired (
				alias TEXT NOT NULL PRIMARY KEY,
				name TEXT,
				revision INTEGER NOT NULL,
				enabled INTEGER NOT NULL
			) STRICT;
			{{ range . }}
			INSERT INTO gatehouse_migration_principal_desired (alias, name, revision, enabled)
			VALUES ({{ sqlLiteral .Alias }}, {{ sqlOptionalString .Name }}, {{ sqlLiteral .Revision }}, {{ sqlBool .Enabled }});
			{{ end }}

			INSERT INTO gatehouse_principals (id, alias, name, revision, enabled)
			SELECT COALESCE(principals.id, gh_id_new('prn')), desired.alias, desired.name, desired.revision, desired.enabled
			FROM gatehouse_migration_principal_desired AS desired
			LEFT JOIN gatehouse_principals AS principals ON principals.alias = desired.alias
			WHERE TRUE
			ON CONFLICT (alias) DO UPDATE SET name = excluded.name, revision = excluded.revision, enabled = excluded.enabled
			WHERE gatehouse_principals.revision < excluded.revision;

			DROP TABLE gatehouse_migration_principal_desired;
		`, principals)
	}
}
