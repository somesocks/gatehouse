<script lang="ts">
  import { onMount } from "svelte"
  import { createApplicationRuntime, provideRuntime } from "./app/runtime.svelte"
  import LegacyApplication from "./pages/legacy/LegacyApplication.svelte"
  import ProjectDashboardRoute from "./pages/project-dashboard/ProjectDashboardRoute.svelte"
  import ProjectNotesRoute from "./pages/project-notes/ProjectNotesRoute.svelte"
  import ProjectSecretsRoute from "./pages/project-secrets/ProjectSecretsRoute.svelte"
  import SessionChatRoute from "./pages/session-chat/SessionChatRoute.svelte"
  import SessionNotesRoute from "./pages/session-notes/SessionNotesRoute.svelte"
  import SessionSecretsRoute from "./pages/session-secrets/SessionSecretsRoute.svelte"
  import GroupsRoute from "./pages/groups/GroupsRoute.svelte"
  import { routeOwner } from "./route-owner"

  const runtime = provideRuntime(createApplicationRuntime())

  onMount(() => {
    runtime.start()
    return () => runtime.stop()
  })
</script>

<svelte:head>
  <meta name="description" content="Gatehouse hosted chat" />
  <title>Gatehouse</title>
</svelte:head>

{#if routeOwner(runtime.state.route) === "project-dashboard"}
  <ProjectDashboardRoute />
{:else if routeOwner(runtime.state.route) === "project-notes"}
  <ProjectNotesRoute />
{:else if routeOwner(runtime.state.route) === "project-secrets"}
  <ProjectSecretsRoute />
{:else if routeOwner(runtime.state.route) === "session-chat"}
  <SessionChatRoute />
{:else if routeOwner(runtime.state.route) === "session-notes"}
  <SessionNotesRoute />
{:else if routeOwner(runtime.state.route) === "session-secrets"}
  <SessionSecretsRoute />
{:else if routeOwner(runtime.state.route) === "groups"}
  <GroupsRoute />
{:else}
  <LegacyApplication />
{/if}
