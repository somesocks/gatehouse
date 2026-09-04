package migrations

import (
	"context"
	"database/sql"
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
		events := make([]activityMigrationEvent, 0, len(workspaces))
		for _, workspace := range workspaces {
			stored, exists := existing[workspace.Alias]
			id := stored.ID
			if !exists {
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
			if !exists || !sameOptionalString(stored.Name, workspace.Name) || stored.Enabled != workspace.Enabled {
				eventName := "workspace.create"
				if exists {
					eventName = "workspace.update"
				}
				event, err := newActivityMigrationEvent(workspace.Alias, eventName, "workspace", "", "", "", id)
				if err != nil {
					return err, ""
				}
				event.WorkspaceID = id
				events = append(events, event)
			}
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range .Values }}
			INSERT INTO gatehouse_workspaces (id, alias, name, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .Name }}, {{ sqlBool .Enabled }})
			ON CONFLICT (alias) DO UPDATE SET
				name = excluded.name,
				enabled = excluded.enabled;
			{{ end }}
			{{ range .Events }}
			{{ $event := . }}
			INSERT INTO gatehouse_activity_events (
				id, event, resource_kind, resource_workspace, created_at
			) VALUES (
				{{ sqlLiteral .ID }}, {{ sqlLiteral .Event }}, {{ sqlLiteral .ResourceKind }}, {{ sqlLiteral .WorkspaceID }}, {{ sqlLiteral .CreatedAt }}
			);
			{{ range .Topics }}
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			VALUES ({{ sqlLiteral $event.ID }}, {{ sqlLiteral . }});
			{{ end }}
			{{ end }}
		`, struct {
			Values []postgresMigrationR003WorkspaceValue
			Events []activityMigrationEvent
		}{Values: values, Events: events})
	}
}

type postgresMigrationR003WorkspaceValue struct {
	ID      string
	Alias   string
	Name    any
	Enabled bool
}

type postgresMigrationR003Workspace struct {
	ID      string
	Name    sql.NullString
	Enabled bool
}

func postgresMigrationR003WorkspaceIDsByAlias(ctx context.Context, session *MigrationSession) (map[string]postgresMigrationR003Workspace, error) {
	rows, err := session.QueryContext(ctx, `SELECT alias, id, name, enabled FROM gatehouse_workspaces WHERE alias IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("get workspace IDs by alias: %w", err)
	}
	defer rows.Close()

	workspaces := map[string]postgresMigrationR003Workspace{}
	for rows.Next() {
		var alias string
		var workspace postgresMigrationR003Workspace
		if err := rows.Scan(&alias, &workspace.ID, &workspace.Name, &workspace.Enabled); err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
		workspaces[alias] = workspace
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspaces: %w", err)
	}
	return workspaces, nil
}
