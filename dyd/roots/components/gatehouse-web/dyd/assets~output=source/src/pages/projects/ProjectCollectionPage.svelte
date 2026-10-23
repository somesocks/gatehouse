<script lang="ts">
  import { RouterLink } from "../../ui"
  import * as UI from "../../ui"
  import { untrack } from "svelte"
  import type { Workspace } from "../../app/access"
  import { createProjectCollectionController } from "./project-collection-controller.svelte"
  import type { ActivityClient } from "../../app/activity"

  let {
    workspace,
    search,
    signal,
    onAuthenticationLost,
    onNavigate,
    activity,
  }: {
    workspace: Workspace
    search: string
    signal: AbortSignal
    onAuthenticationLost: () => void
    onNavigate: (path: string) => void
    activity?: ActivityClient
  } = $props()
  const controller = untrack(() =>
    createProjectCollectionController({ onAuthenticationLost, activity }),
  )

  $effect(() => {
    const workspaceID = workspace.id
    const routeSearch = search
    const routeSignal = signal
    return untrack(() =>
      controller.start(workspaceID, routeSearch, routeSignal),
    )
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

{#snippet projectActions()}<UI.RouterLink
    variant="default"
    size="sm"
    href={`${projectsPath()}/new`}>New project</UI.RouterLink
  >{/snippet}
<UI.AdminContent>
  <UI.AdminHeader
    title="Projects"
    description="Organize shared work and project resources."
    actions={projectActions}
  />
  <UI.SearchToolbar
    label="Search projects"
    placeholder="Project name"
    bind:value={controller.state.search}
    loading={controller.state.busy}
    onsearch={() =>
      controller.state.search.trim() === search.trim()
        ? void controller.search()
        : onNavigate(searchPath())}
  />
  {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
      >{controller.state.error}</UI.Notice
    >{/if}
  {#if controller.state.reloading}<UI.Text role="status"
      >Refreshing projects…</UI.Text
    >{/if}
  {#if controller.state.error}<UI.Button
      variant="outline"
      onclick={() => void controller.retry()}>Try again</UI.Button
    >{/if}
  <UI.RecordTable headers={["Name", "Created"]}>
    {#each controller.state.projects as project (project.id)}<UI.RecordRow>
        <UI.RecordCell primary
          ><RouterLink href={projectPath(project.id)}
            >{project.name ?? "Untitled project"}</RouterLink
          ></UI.RecordCell
        >
        <UI.RecordCell
          ><UI.Text as="time" datetime={project.created_at}
            >{createdAtLabel(project.created_at)}</UI.Text
          ></UI.RecordCell
        >
      </UI.RecordRow>{:else}<UI.TableEmpty
        columns={2}
        message={!controller.state.hasLoaded && controller.state.error
          ? "Projects could not be loaded."
          : controller.state.loading
            ? "Searching projects…"
            : "No projects match your search."}
      />{/each}
  </UI.RecordTable>
  <UI.PaginationFooter
    noun="projects"
    count={controller.state.projects.length}
    previous={controller.state.history.length > 0}
    next={controller.state.cursor !== null}
    loading={controller.state.busy}
    onprevious={() => controller.previous(workspace.id, search, signal)}
    onnext={() => controller.loadMore(workspace.id, search, signal)}
  />
</UI.AdminContent>
