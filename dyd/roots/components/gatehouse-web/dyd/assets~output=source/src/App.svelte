<script lang="ts">
  import { onMount } from "svelte"
  import { createApplicationRuntime, provideRuntime } from "./app/runtime.svelte"
  import LegacyApplication from "./pages/legacy/LegacyApplication.svelte"
  import ProjectDashboardRoute from "./pages/project-dashboard/ProjectDashboardRoute.svelte"
  import ProjectNotesRoute from "./pages/project-notes/ProjectNotesRoute.svelte"
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
{:else}
  <LegacyApplication />
{/if}
