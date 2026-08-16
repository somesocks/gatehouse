package migrations

import (
	"context"
	"fmt"

	"gatehouse/config"
	"gatehouse/typed_id"
)

func workspaceMigrationBuilder(workspaces []config.Workspace) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		existing, err := workspaceIDsByKey(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]workspaceMigrationValue, 0, len(workspaces))
		for _, workspace := range workspaces {
			id := existing[workspace.Key]
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
			values = append(values, workspaceMigrationValue{ID: id, Key: workspace.Key, Name: name, Enabled: workspace.Enabled})
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_workspaces (id, key, name, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Key }}, {{ sqlLiteral .Name }}, {{ sqlBool .Enabled }})
			ON CONFLICT (key) DO UPDATE SET
				name = excluded.name,
				enabled = excluded.enabled;
			{{ end }}
		`, values)
	}
}

func workspaceIDsByKey(ctx context.Context, session *MigrationSession) (map[string]string, error) {
	rows, err := session.QueryContext(ctx, `SELECT key, id FROM gatehouse_workspaces`)
	if err != nil {
		return nil, fmt.Errorf("get workspace IDs: %w", err)
	}
	defer rows.Close()
	ids := map[string]string{}
	for rows.Next() {
		var key, id string
		if err := rows.Scan(&key, &id); err != nil {
			return nil, fmt.Errorf("scan workspace ID: %w", err)
		}
		ids[key] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace IDs: %w", err)
	}
	return ids, nil
}
