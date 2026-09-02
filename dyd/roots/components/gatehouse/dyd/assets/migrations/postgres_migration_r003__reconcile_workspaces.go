package migrations

import (
	"context"
	"fmt"

	"gatehouse/config"
	"gatehouse/typed_id"
)

func postgresMigrationR003ReconcileWorkspaces(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       3,
		Description: "reconcile_workspaces",
		Builder:     postgresMigrationR003ReconcileWorkspacesBuilder(state.Workspaces),
	}
}

func postgresMigrationR003ReconcileWorkspacesBuilder(workspaces []config.Workspace) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		existing, err := postgresMigrationR003WorkspaceIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]postgresMigrationR003WorkspaceValue, 0, len(workspaces))
		for _, workspace := range workspaces {
			id := existing[workspace.Alias]
			if id == "" {
				id, err = typed_id.New(typed_id.Workspace)
				if err != nil {
					return err, ""
				}
			}
			var name any
			if workspace.Name != nil {
				name = *workspace.Name
			}
			values = append(values, postgresMigrationR003WorkspaceValue{ID: id, Alias: workspace.Alias, Name: name, Enabled: workspace.Enabled})
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_workspaces (id, alias, name, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .Name }}, {{ sqlBool .Enabled }})
			ON CONFLICT (alias) DO UPDATE SET
				name = excluded.name,
				enabled = excluded.enabled;
			{{ end }}
		`, values)
	}
}

type postgresMigrationR003WorkspaceValue struct {
	ID      string
	Alias   string
	Name    any
	Enabled bool
}

func postgresMigrationR003WorkspaceIDsByAlias(ctx context.Context, session *MigrationSession) (map[string]string, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, id FROM gatehouse_workspaces WHERE alias IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("get workspace IDs by alias: %w", err)
	}
	defer rows.Close()

	ids := map[string]string{}
	for rows.Next() {
		var alias, id string
		if err := rows.Scan(&alias, &id); err != nil {
			return nil, fmt.Errorf("scan workspace ID: %w", err)
		}
		ids[alias] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace IDs: %w", err)
	}
	return ids, nil
}
