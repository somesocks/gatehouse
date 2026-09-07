<script lang="ts">
  import { onMount } from "svelte"
  import { createApplicationRuntime, provideRuntime } from "./app/runtime.svelte"
  import AccessRoute from "./pages/access/AccessRoute.svelte"
  import AgentProvidersRoute from "./pages/system/agent-providers/AgentProvidersRoute.svelte"
  import AgentModelsRoute from "./pages/system/agent-models/AgentModelsRoute.svelte"
  import ProjectDashboardRoute from "./pages/project-dashboard/ProjectDashboardRoute.svelte"
  import ProjectNotesRoute from "./pages/project-notes/ProjectNotesRoute.svelte"
  import ProjectSecretsRoute from "./pages/project-secrets/ProjectSecretsRoute.svelte"
  import SessionChatRoute from "./pages/session-chat/SessionChatRoute.svelte"
  import SessionNotesRoute from "./pages/session-notes/SessionNotesRoute.svelte"
  import SessionSecretsRoute from "./pages/session-secrets/SessionSecretsRoute.svelte"
  import GroupsRoute from "./pages/groups/GroupsRoute.svelte"
  import ChatCollectionRoute from "./pages/chats/ChatCollectionRoute.svelte"
  import ProjectCollectionRoute from "./pages/projects/ProjectCollectionRoute.svelte"
  import SystemRoute from "./pages/system/SystemRoute.svelte"
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
{:else if owner === "agent-providers"}
  <AgentProvidersRoute />
{:else if owner === "agent-models"}
  <AgentModelsRoute />
{:else if owner === "chat-collection"}
  <ChatCollectionRoute />
{:else if owner === "project-collection"}
  <ProjectCollectionRoute />
{:else if owner === "system"}
  <SystemRoute />
{:else if owner === "workspace-dashboard"}
  <WorkspaceDashboardRoute />
{:else if owner === "access"}
  <AccessRoute />
{/if}
