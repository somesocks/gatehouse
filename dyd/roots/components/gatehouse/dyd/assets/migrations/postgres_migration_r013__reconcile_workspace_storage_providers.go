package migrations

import (
	"context"
	"fmt"

	"gatehouse/config"
)

func postgresMigrationR013ReconcileWorkspaceStorageProviders(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       13,
		Description: "reconcile_workspace_storage_providers",
		Builder:     postgresMigrationR013ReconcileWorkspaceStorageProvidersBuilder(state.WorkspaceStorageProviders),
	}
}

type postgresMigrationR013WorkspaceStorageProviderAliasKey struct {
	Workspace string
	Provider  string
}

type postgresMigrationR013StoredWorkspaceStorageProvider struct {
	Revision int
	Priority int
	Enabled  bool
}

func postgresMigrationR013ReconcileWorkspaceStorageProvidersBuilder(providers []config.WorkspaceStorageProvider) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		existing, err := postgresMigrationR013WorkspaceStorageProvidersByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		workspaceIDs, err := workspaceIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		storageProviders, err := postgresMigrationR012StorageProvidersByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		desired := make([]config.WorkspaceStorageProvider, 0, len(providers))
		events := make([]activityMigrationEvent, 0, len(providers))
		for _, provider := range providers {
			key := postgresMigrationR013WorkspaceStorageProviderAliasKey{Workspace: provider.WorkspaceID, Provider: provider.ProviderAlias}
			stored, exists := existing[key]
			if exists && stored.Revision >= provider.Revision {
				continue
			}
			desired = append(desired, provider)
			workspaceID := workspaceIDs[provider.WorkspaceID]
			if workspaceID == "" {
				return fmt.Errorf("workspace %q is unavailable", provider.WorkspaceID), ""
			}
			storageProviderID := storageProviders[provider.ProviderAlias].ID
			if storageProviderID == "" {
				return fmt.Errorf("storage provider %q is unavailable", provider.ProviderAlias), ""
			}
			eventName := "workspace_storage_provider.create"
			if exists {
				eventName = "workspace_storage_provider.update"
			}
			event, err := newActivityMigrationEvent(provider.WorkspaceID, eventName, "workspace_storage_provider", "", "", "", workspaceID)
			if err != nil {
				return err, ""
			}
			event.WorkspaceStorageProviderWorkspace = workspaceID
			event.WorkspaceStorageProviderProvider = storageProviderID
			events = append(events, event)
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range .Providers }}
			INSERT INTO gatehouse_workspace_storage_providers (workspace, provider, revision, priority, enabled)
				VALUES ((SELECT id FROM gatehouse_workspaces WHERE alias = {{ sqlLiteral .WorkspaceID }}), (SELECT id FROM gatehouse_storage_providers WHERE alias = {{ sqlLiteral .ProviderAlias }}), {{ sqlLiteral .Revision }}, {{ sqlLiteral .Priority }}, {{ sqlBool .Enabled }})
			ON CONFLICT (workspace, provider) DO UPDATE SET revision = excluded.revision, priority = excluded.priority, enabled = excluded.enabled
			WHERE gatehouse_workspace_storage_providers.revision < excluded.revision;
			{{ end }}
			{{ range .Events }}
			{{ $event := . }}
			INSERT INTO gatehouse_activity_events (
				id, event, resource_kind, resource_workspace_storage_provider_workspace, resource_workspace_storage_provider_provider, created_at
			) VALUES (
				{{ sqlLiteral .ID }}, {{ sqlLiteral .Event }}, {{ sqlLiteral .ResourceKind }}, {{ sqlLiteral .WorkspaceStorageProviderWorkspace }}, {{ sqlLiteral .WorkspaceStorageProviderProvider }}, {{ sqlLiteral .CreatedAt }}
			);
			{{ range .Topics }}
			INSERT INTO gatehouse_activity_event_topics (activity, topic)
			VALUES ({{ sqlLiteral $event.ID }}, {{ sqlLiteral . }});
			{{ end }}
			{{ end }}
		`, struct {
			Providers []config.WorkspaceStorageProvider
			Events    []activityMigrationEvent
		}{Providers: desired, Events: events})
	}
}

func postgresMigrationR013WorkspaceStorageProvidersByAlias(ctx context.Context, session *MigrationSession) (map[postgresMigrationR013WorkspaceStorageProviderAliasKey]postgresMigrationR013StoredWorkspaceStorageProvider, error) {
	rows, err := session.QueryContext(ctx, `
		SELECT workspaces.alias, providers.alias, bindings.revision, bindings.priority, bindings.enabled
		FROM gatehouse_workspace_storage_providers AS bindings
		JOIN gatehouse_workspaces AS workspaces ON workspaces.id = bindings.workspace
		JOIN gatehouse_storage_providers AS providers ON providers.id = bindings.provider
		WHERE workspaces.alias IS NOT NULL AND providers.alias IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("get workspace storage providers by alias: %w", err)
	}
	defer rows.Close()

	bindings := map[postgresMigrationR013WorkspaceStorageProviderAliasKey]postgresMigrationR013StoredWorkspaceStorageProvider{}
	for rows.Next() {
		var key postgresMigrationR013WorkspaceStorageProviderAliasKey
		var binding postgresMigrationR013StoredWorkspaceStorageProvider
		if err := rows.Scan(&key.Workspace, &key.Provider, &binding.Revision, &binding.Priority, &binding.Enabled); err != nil {
			return nil, fmt.Errorf("scan workspace storage provider: %w", err)
		}
		bindings[key] = binding
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace storage providers: %w", err)
	}
	return bindings, nil
}
