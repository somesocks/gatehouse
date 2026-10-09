<script lang="ts">
  import { Search, PageBody, PageHeading, RouterLink } from "../../ui"
  import * as UI from "../../ui"
  import { untrack } from "svelte"
  import type { Workspace } from "../../app/access"
  import { createProjectCollectionController } from "./project-collection-controller.svelte"

  let {
    workspace,
    search,
    signal,
    creating,
    onAuthenticationLost,
    onCreate,
    onNavigate,
  }: {
    workspace: Workspace
    search: string
    signal: AbortSignal
    creating: boolean
    onAuthenticationLost: () => void
    onCreate: () => void | Promise<void>
    onNavigate: (path: string) => void
  } = $props()
  const controller = untrack(() =>
    createProjectCollectionController({ onAuthenticationLost }),
  )

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
    return query === ""
      ? projectsPath()
      : `${projectsPath()}?${new URLSearchParams({ name: query })}`
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

{#snippet projectActions()}<UI.Button
    variant="default"
    size="sm"
    type="button"
    disabled={creating}
    onclick={() => void onCreate()}>New project</UI.Button
  >{/snippet}
<PageBody fluid>
  <PageHeading actions={projectActions}>
    <UI.Heading level={1}>Projects</UI.Heading>
  </PageHeading>
  <UI.Form
    layout="search"
    onsubmit={(event) => {
      event.preventDefault()
      onNavigate(searchPath())
    }}
  >
    <UI.Label layout="field"
      ><UI.Text>Search projects</UI.Text><UI.Input
        type="search"
        autocomplete="off"
        placeholder="Search projects"
        bind:value={controller.state.search}
      /></UI.Label
    >
    <UI.Button
      variant="ghost"
      size="icon"
      type="submit"
      aria-label="Search projects"
      title="Search projects"
      ><Search size={20} strokeWidth={2} aria-hidden="true" /></UI.Button
    >
  </UI.Form>
  <UI.List>
    {#each controller.state.projects as project}
      <RouterLink variant="item" href={projectPath(project.id)}
        ><UI.Stack as="span" gap="tight"
          ><UI.Text>{project.name ?? "New Project"}</UI.Text><UI.Text
            as="time"
            datetime={project.created_at}
            >{createdAtLabel(project.created_at)}</UI.Text
          ></UI.Stack
        ></RouterLink
      >
    {:else}<UI.Notice>
        {controller.state.loading
          ? "Searching projects..."
          : "No projects match your search."}
      </UI.Notice>{/each}
  </UI.List>
  {#if controller.state.cursor !== null}<UI.Button
      variant="outline"
      size="sm"
      type="button"
      disabled={controller.state.loading}
      onclick={() => controller.loadMore(workspace.id, search, signal)}
      >{controller.state.loading ? "Loading..." : "Show more"}</UI.Button
    >{/if}
</PageBody>
