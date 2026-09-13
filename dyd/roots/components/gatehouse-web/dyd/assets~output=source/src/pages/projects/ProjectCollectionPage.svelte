<script lang="ts">
  import { untrack } from "svelte"
  import { Search } from "@lucide/svelte"
  import type { Workspace } from "../../app/access"
  import RouterLink from "../../components/RouterLink.svelte"
  import { createProjectCollectionController } from "./project-collection-controller.svelte"

  let { workspace, search, signal, creating, onAuthenticationLost, onCreate, onNavigate }: {
    workspace: Workspace
    search: string
    signal: AbortSignal
    creating: boolean
    onAuthenticationLost: () => void
    onCreate: () => void | Promise<void>
    onNavigate: (path: string) => void
  } = $props()
  const controller = untrack(() => createProjectCollectionController({ onAuthenticationLost }))

  $effect(() => {
    const workspaceID = workspace.id
    const routeSearch = search
    return untrack(() => controller.start(workspaceID, routeSearch, signal))
  })

  function projectsPath() {
    return `/app/wsp/${encodeURIComponent(workspace.id)}/prj`
  }

  function projectPath(id: string) {
    return `${projectsPath()}/${encodeURIComponent(id)}`
  }

  function searchPath() {
    const query = controller.state.search.trim()
    return query === "" ? projectsPath() : `${projectsPath()}?${new URLSearchParams({ name: query })}`
  }

  function createdAtLabel(value: string) {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) {
      return value
    }
    const number = (part: number) => part.toString().padStart(2, "0")
    return `${date.getFullYear()}-${number(date.getMonth() + 1)}-${number(date.getDate())} ${number(date.getHours())}:${number(date.getMinutes())}`
  }
</script>

<section class="collection-page">
  <div class="collection-heading"><h1 class="brand-dashboard-title">Projects</h1><button class="button is-primary is-small" type="button" disabled={creating} onclick={() => void onCreate()}>New project</button></div>
  <form class="collection-search" onsubmit={(event) => { event.preventDefault(); onNavigate(searchPath()) }}>
    <label><span>Search projects</span><input class="input" type="search" autocomplete="off" placeholder="Search projects" bind:value={controller.state.search} /></label>
    <button class="button" type="submit" aria-label="Search projects" title="Search projects"><Search size={20} strokeWidth={2} aria-hidden="true" /></button>
  </form>
  <div class="collection-list">
    {#each controller.state.projects as project}
      <RouterLink class="dashboard-row" href={projectPath(project.id)}><span class="dashboard-row-content"><span>{project.name ?? "New Project"}</span><time datetime={project.created_at}>{createdAtLabel(project.created_at)}</time></span></RouterLink>
    {:else}<p class="dashboard-empty">{controller.state.loading ? "Searching projects..." : "No projects match your search."}</p>{/each}
  </div>
  {#if controller.state.cursor !== null}<button class="button is-small" type="button" disabled={controller.state.loading} onclick={() => controller.loadMore(workspace.id, search, signal)}>{controller.state.loading ? "Loading..." : "Show more"}</button>{/if}
</section>
