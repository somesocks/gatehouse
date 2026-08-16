package migrations

import (
	"context"
	"fmt"

	"gatehouse/config"
	"gatehouse/typed_id"
)

func workspaceMigrationBuilder(workspaces []config.Workspace) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		existing, err := workspaceIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]workspaceMigrationValue, 0, len(workspaces))
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
			values = append(values, workspaceMigrationValue{ID: id, Alias: workspace.Alias, Name: name, Enabled: workspace.Enabled})
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

func workspaceIDsByAlias(ctx context.Context, session *MigrationSession) (map[string]string, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, id FROM gatehouse_workspaces`)
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
