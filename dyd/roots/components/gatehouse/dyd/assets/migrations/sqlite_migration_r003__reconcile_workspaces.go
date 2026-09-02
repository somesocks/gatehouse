package migrations

import (
	"context"

	"gatehouse/config"
)

func sqliteMigrationR003ReconcileWorkspaces(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       3,
		Description: "reconcile_workspaces",
		Builder:     sqliteMigrationR003ReconcileWorkspacesBuilder(state.Workspaces),
	}
}

func sqliteMigrationR003ReconcileWorkspacesBuilder(workspaces []config.Workspace) MigrationBuilder {
	return func(_ context.Context, session *MigrationSession) (error, string) {
		return session.RenderTemplate(`
			DROP TABLE IF EXISTS gatehouse_migration_workspace_desired;

			CREATE TEMP TABLE gatehouse_migration_workspace_desired (
				alias TEXT NOT NULL PRIMARY KEY,
				name TEXT,
				enabled INTEGER NOT NULL
			) STRICT;
			{{ range . }}
			INSERT INTO gatehouse_migration_workspace_desired (alias, name, enabled)
			VALUES ({{ sqlLiteral .Alias }}, {{ sqlOptionalString .Name }}, {{ sqlBool .Enabled }});
			{{ end }}

			INSERT INTO gatehouse_workspaces (id, alias, name, enabled)
			SELECT COALESCE(workspaces.id, gh_id_new('wsp')), desired.alias, desired.name, desired.enabled
			FROM gatehouse_migration_workspace_desired AS desired
			LEFT JOIN gatehouse_workspaces AS workspaces ON workspaces.alias = desired.alias
			WHERE TRUE
			ON CONFLICT (alias) DO UPDATE SET name = excluded.name, enabled = excluded.enabled;

			DROP TABLE gatehouse_migration_workspace_desired;
		`, workspaces)
	}
}
