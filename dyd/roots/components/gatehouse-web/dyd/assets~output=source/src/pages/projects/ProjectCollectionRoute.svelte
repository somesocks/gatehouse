<script lang="ts">
  import { createProject, type Project } from "../../app/projects"
  import { useRuntime } from "../../app/runtime.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import StatusPage from "../../components/StatusPage.svelte"
  import * as SidebarPage from "../../components/sidebar-page"
  import WorkspaceNavigation from "../../components/WorkspaceNavigation.svelte"
  import type { Route } from "../../route"
  import ProjectCollectionPage from "./ProjectCollectionPage.svelte"

  type ProjectCollectionRoute = Extract<Route, { kind: "project-collection" }>

  const runtime = useRuntime()
  const { access, auth } = runtime
  let creating = $state(false)
  let generation = 0
  let abortController: AbortController | null = null
  let routeSignal = $state<AbortSignal | null>(null)

  const currentRoute = $derived(runtime.state.route as ProjectCollectionRoute)
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const workspacePath = (workspaceID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}`
  const projectsPath = (workspaceID: string) =>
    `${workspacePath(workspaceID)}/prj`
  const isCurrent = (value: number, workspaceID: string, signal: AbortSignal) =>
    value === generation &&
    !signal.aborted &&
    currentRoute.workspaceID === workspaceID

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })

  function activate(route: ProjectCollectionRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    routeSignal = abortController.signal
    creating = false
    if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "ready"
    )
      validateWorkspace(route, value, abortController.signal)
    else if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "checking"
    )
      void runtime.refresh()
    return () => {
      if (value === generation) abortController?.abort()
    }
  }

  function validateWorkspace(
    route: ProjectCollectionRoute,
    value: number,
    signal: AbortSignal,
  ): void {
    if (
      access.state.workspaces.some(
        (candidate) => candidate.id === route.workspaceID,
      )
    )
      return
    if (isCurrent(value, route.workspaceID, signal))
      runtime.navigate(
        access.state.workspaces.length === 0
          ? "/app/no-access"
          : workspacePath(access.state.workspaces[0].id),
        true,
      )
  }

  async function create(): Promise<void> {
    if (creating) return
    const route = currentRoute
    const value = generation
    const signal = abortController?.signal
    if (signal === undefined) return
    creating = true
    try {
      const response = await createProject(route.workspaceID, signal)
      if (!isCurrent(value, route.workspaceID, signal)) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("project could not be created")
      const project = (await response.json()) as Project
      if (!isCurrent(value, route.workspaceID, signal)) return
      runtime.navigate(
        `${projectsPath(route.workspaceID)}/${encodeURIComponent(project.id)}`,
      )
    } catch {
      // The existing collection has no error surface for failed creation.
    } finally {
      if (isCurrent(value, route.workspaceID, signal)) creating = false
    }
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <StatusPage eyebrow="Gatehouse" busy live="polite">
    {#snippet children()}
      <p>
        {auth.state.status === "checking"
          ? "Checking your session."
          : "Loading your workspaces."}
      </p>
    {/snippet}
  </StatusPage>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage eyebrow="Gatehouse" title="Connection unavailable" description="Gatehouse could not load your account.">
    {#snippet children()}<div><button class="primary" type="button" onclick={() => void runtime.refresh()}>Try again</button></div>{/snippet}
  </StatusPage>
{:else if auth.state.status !== "authenticated"}
  <StatusPage eyebrow="Gatehouse" title="Sign in required">
    {#snippet children()}<div><button class="primary" type="button" onclick={() => runtime.requireLogin()}>Sign in</button></div>{/snippet}
  </StatusPage>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <StatusPage eyebrow="Gatehouse" title="No workspace access" description="Ask an administrator to add you to a workspace group.">
    {#snippet children()}<div><button class="primary" type="button" onclick={() => runtime.navigate("/app/no-access", true)}>Continue</button></div>{/snippet}
  </StatusPage>
{:else}
  <SidebarPage.Root
    ><SidebarPage.Sidebar
      ><WorkspaceNavigation
        {workspace}
        active="projects"
      /></SidebarPage.Sidebar
    ><SidebarPage.Page
      ><SidebarPage.Header
        ><SidebarPage.Toggle />
        <nav aria-label="Breadcrumb"><ol><li><RouterLink href={workspacePath(workspace.id)}>{workspace.name ?? workspace.id}</RouterLink></li><li aria-current="page">Projects</li></ol></nav></SidebarPage.Header
      ><SidebarPage.Body
        >{#if routeSignal !== null}<ProjectCollectionPage
            {workspace}
            search={currentRoute.search}
            signal={routeSignal}
            {creating}
            onAuthenticationLost={() => runtime.requireLogin()}
            onCreate={create}
            onNavigate={runtime.navigate}
          />{/if}</SidebarPage.Body
      ></SidebarPage.Page
    ></SidebarPage.Root
  >
{/if}
