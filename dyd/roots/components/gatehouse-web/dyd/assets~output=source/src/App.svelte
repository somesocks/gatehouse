<script lang="ts">
  import { onMount } from "svelte"
  import { createApplicationRuntime, provideRuntime } from "./app/runtime.svelte"
  import AccessRoute from "./pages/access/AccessRoute.svelte"
  import AgentProviderDetailRoute from "./pages/system/agent-providers/AgentProviderDetailRoute.svelte"
  import AgentProviderListRoute from "./pages/system/agent-providers/AgentProviderListRoute.svelte"
  import AgentProviderNewRoute from "./pages/system/agent-providers/AgentProviderNewRoute.svelte"
  import AgentModelDetailRoute from "./pages/system/agent-models/AgentModelDetailRoute.svelte"
  import AgentModelListRoute from "./pages/system/agent-models/AgentModelListRoute.svelte"
  import AgentModelNewRoute from "./pages/system/agent-models/AgentModelNewRoute.svelte"
  import StorageProvidersRoute from "./pages/system/storage-providers/StorageProvidersRoute.svelte"
  import PrincipalsRoute from "./pages/system/principals/PrincipalsRoute.svelte"
  import SystemGrantsRoute from "./pages/system/grants/SystemGrantsRoute.svelte"
  import SystemOverviewRoute from "./pages/system/overview/SystemOverviewRoute.svelte"
  import WorkspaceAgentBindingsRoute from "./pages/system/workspace-agent-bindings/WorkspaceAgentBindingsRoute.svelte"
  import WorkspaceStorageBindingsRoute from "./pages/system/workspace-storage-bindings/WorkspaceStorageBindingsRoute.svelte"
  import ProjectDashboardRoute from "./pages/project-dashboard/ProjectDashboardRoute.svelte"
  import ProjectNotesRoute from "./pages/project-notes/ProjectNotesRoute.svelte"
  import ProjectSecretsRoute from "./pages/project-secrets/ProjectSecretsRoute.svelte"
  import SessionChatRoute from "./pages/session-chat/SessionChatRoute.svelte"
  import SessionNotesRoute from "./pages/session-notes/SessionNotesRoute.svelte"
  import SessionSecretsRoute from "./pages/session-secrets/SessionSecretsRoute.svelte"
  import GroupsRoute from "./pages/groups/GroupsRoute.svelte"
  import ChatCollectionRoute from "./pages/chats/ChatCollectionRoute.svelte"
  import ProjectCollectionRoute from "./pages/projects/ProjectCollectionRoute.svelte"
  import WorkspaceDashboardRoute from "./pages/workspace/WorkspaceDashboardRoute.svelte"
  import { routeOwner } from "./route-owner"

  const runtime = provideRuntime(createApplicationRuntime())
  const owner = $derived(routeOwner(runtime.state.route))

  onMount(() => {
    runtime.start()
    return () => runtime.stop()
  })
</script>

<svelte:head>
  <meta name="description" content="Gatehouse hosted chat" />
  <title>Gatehouse</title>
</svelte:head>

{#if owner === "project-dashboard"}
  <ProjectDashboardRoute />
{:else if owner === "project-notes"}
  <ProjectNotesRoute />
{:else if owner === "project-secrets"}
  <ProjectSecretsRoute />
{:else if owner === "session-chat"}
  <SessionChatRoute />
{:else if owner === "session-notes"}
  <SessionNotesRoute />
{:else if owner === "session-secrets"}
  <SessionSecretsRoute />
{:else if owner === "groups"}
  <GroupsRoute />
{:else if owner === "agent-provider-list"}
  <AgentProviderListRoute />
{:else if owner === "agent-provider-new"}
  <AgentProviderNewRoute />
{:else if owner === "agent-provider-detail"}
  <AgentProviderDetailRoute providerID={runtime.state.route.kind === "system-agent-provider" ? runtime.state.route.providerID : ""} />
{:else if owner === "agent-model-list"}
  <AgentModelListRoute />
{:else if owner === "agent-model-new"}
  <AgentModelNewRoute />
{:else if owner === "agent-model-detail"}
  <AgentModelDetailRoute modelID={runtime.state.route.kind === "system-agent-model" ? runtime.state.route.modelID : ""} />
{:else if owner === "storage-providers"}
  <StorageProvidersRoute />
{:else if owner === "principals"}
  <PrincipalsRoute />
{:else if owner === "system-grants"}
  <SystemGrantsRoute />
{:else if owner === "system-overview"}
  <SystemOverviewRoute />
{:else if owner === "workspace-agent-bindings"}
  <WorkspaceAgentBindingsRoute />
{:else if owner === "workspace-storage-bindings"}
  <WorkspaceStorageBindingsRoute />
{:else if owner === "chat-collection"}
  <ChatCollectionRoute />
{:else if owner === "project-collection"}
  <ProjectCollectionRoute />
{:else if owner === "workspace-dashboard"}
  <WorkspaceDashboardRoute />
{:else if owner === "access"}
  <AccessRoute />
{/if}
