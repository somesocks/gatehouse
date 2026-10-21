<script lang="ts">
  import { untrack } from "svelte"
  import { createLoadState } from "../../utils/load-state.svelte"
  import { RouterLink, StatusPage } from "../../ui"
  import * as UI from "../../ui"
  import * as SidebarPage from "../../ui/sidebar-page"
  import { workspaceNavigation } from "../../app/navigation"
  import { fetchProject, type Project } from "../../app/projects"
  import { useRuntime } from "../../app/runtime.svelte"
  import type { Route } from "../../route"

  type DashboardRoute = Extract<Route, { kind: "project" }>
  type Status = "checking" | "ready" | "unavailable"
  const runtime = useRuntime()
  const resource = createLoadState()
  const { activity, access, auth } = runtime
  let project = $state<Project | null>(null)
  const projectStatus = $derived(resource.status)
  let generation = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined
  const currentRoute = $derived(runtime.state.route as DashboardRoute)
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const projectPath = (workspaceID: string, projectID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}/prj/${encodeURIComponent(projectID)}`
  const base = $derived(
    projectPath(currentRoute.workspaceID, currentRoute.projectID),
  )
  const isCurrent = (value: number, workspaceID: string, projectID: string) =>
    value === generation &&
    currentRoute.workspaceID === workspaceID &&
    currentRoute.projectID === projectID &&
    !abortController?.signal.aborted

  const activation = $derived(
    JSON.stringify([
      currentRoute,
      auth.state.status,
      auth.state.claims?.principal.ref.id,
      access.state.workspaceStatus,
      workspace !== null,
    ]),
  )
  $effect(() => {
    activation
    return untrack(() => activate(currentRoute))
  })

  function activate(route: DashboardRoute): () => void {
    const value = ++generation
    resource.reset()
    abortController?.abort()
    const controller = new AbortController()
    abortController = controller
    unsubscribe?.()
    unsubscribe = undefined
    project = null
    if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "ready"
    ) {
      void loadRoute(route, value, controller.signal)
    } else if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "checking"
    ) {
      void runtime.refresh()
    }
    return () => {
      controller.abort()
      resource.stop()
      if (value === generation) {
        unsubscribe?.()
        unsubscribe = undefined
      }
    }
  }

  async function loadRoute(
    route: DashboardRoute,
    value: number,
    signal: AbortSignal,
  ) {
    const { workspaceID, projectID } = route
    if (
      !access.state.workspaces.some((candidate) => candidate.id === workspaceID)
    ) {
      if (isCurrent(value, workspaceID, projectID))
        runtime.navigate(
          access.state.workspaces.length === 0
            ? "/app/no-access"
            : `/app/wsp/${encodeURIComponent(access.state.workspaces[0].id)}`,
          true,
        )
      return
    }
    if (
      !(await loadProject(value, workspaceID, projectID, signal)) ||
      !isCurrent(value, workspaceID, projectID)
    )
      return
    subscribe(route, value)
  }

  async function loadProject(
    value: number,
    workspaceID: string,
    projectID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    const request = resource.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
    try {
      const response = await fetchProject(workspaceID, projectID, signal)
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (response.status === 404) {
        runtime.navigate(
          `/app/wsp/${encodeURIComponent(workspaceID)}/prj`,
          true,
        )
        return false
      }
      if (!response.ok) throw new Error("project unavailable")
      const loaded = (await response.json()) as Project
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      return request.commit(() => {
        project = loaded
      })
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        request.fail(new Error("Project could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  function subscribe(route: DashboardRoute, value: number) {
    const { workspaceID, projectID } = route
    unsubscribe?.()
    unsubscribe = activity.subscribe(
      [
        {
          name: "project-dashboard",
          topic: `${workspaceID}/${projectID}`,
          events: ["project.*"],
        },
      ],
      async ({ signal }) => {
        if (!isCurrent(value, workspaceID, projectID) || signal.aborted) return
        if (
          !(await loadProject(value, workspaceID, projectID, signal, false)) ||
          signal.aborted
        )
          throw new Error("project dashboard refresh failed")
      },
    )
    void activity.poll()
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <StatusPage eyebrow="Gatehouse" busy live="polite">
    <UI.Text as="p"
      >{auth.state.status === "checking"
        ? "Checking your session."
        : "Loading your workspaces."}</UI.Text
    >
  </StatusPage>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage
    eyebrow="Gatehouse"
    title="Connection unavailable"
    description="Gatehouse could not load your account."
  >
    <UI.Group
      ><UI.Button onclick={() => void runtime.refresh()}>Try again</UI.Button
      ></UI.Group
    >
  </StatusPage>
{:else if auth.state.status !== "authenticated"}
  <StatusPage eyebrow="Gatehouse" title="Sign in required"
    ><UI.Group
      ><UI.Button onclick={() => runtime.requireLogin()}>Sign in</UI.Button
      ></UI.Group
    ></StatusPage
  >
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <StatusPage
    eyebrow="Gatehouse"
    title="No workspace access"
    description="Ask an administrator to add you to a workspace group."
    ><UI.Group
      ><UI.Button onclick={() => runtime.navigate("/app/no-access", true)}
        >Continue</UI.Button
      ></UI.Group
    ></StatusPage
  >
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar
      ><UI.Navigation
        {...workspaceNavigation(runtime, workspace, "projects")}
      /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header>
        <SidebarPage.Toggle />
        <UI.Breadcrumb.Root aria-label="Breadcrumb"
          ><UI.Breadcrumb.List>
            <UI.Breadcrumb.Item
              ><RouterLink href={`/app/wsp/${encodeURIComponent(workspace.id)}`}
                >{workspace.name ?? "Workspace"}</RouterLink
              ></UI.Breadcrumb.Item
            >
            <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
              ><RouterLink
                href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj`}
                >Projects</RouterLink
              ></UI.Breadcrumb.Item
            >
            <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
              ><UI.Breadcrumb.Page
                >{project?.name ?? "Project"}</UI.Breadcrumb.Page
              ></UI.Breadcrumb.Item
            >
          </UI.Breadcrumb.List></UI.Breadcrumb.Root
        >
      </SidebarPage.Header>
      <SidebarPage.Body>
        <UI.AdminContent mode="hub">
          {#if resource.reloading}<UI.Text role="status"
              >Refreshing project…</UI.Text
            >{/if}
          {#if resource.state.error}<UI.Notice
              variant="destructive"
              role="alert"
              >{resource.state.error}<UI.Button
                variant="outline"
                onclick={() =>
                  void loadRoute(
                    currentRoute,
                    generation,
                    abortController!.signal,
                  )}>Retry</UI.Button
              ></UI.Notice
            >{/if}
          {#if projectStatus === "checking"}<UI.Notice
              ><UI.Text as="p">Loading project…</UI.Text></UI.Notice
            >
          {:else if projectStatus === "unavailable"}<UI.Notice
              ><UI.Text as="p">Project unavailable.</UI.Text><UI.Button
                onclick={() =>
                  void loadRoute(
                    currentRoute,
                    generation,
                    abortController!.signal,
                  )}>Try again</UI.Button
              ></UI.Notice
            >
          {:else if project}
            <UI.AdminHeader
              kind="Project"
              title={project.name ?? "Untitled project"}
              description={project.description}
            >
              {#snippet actions()}<UI.RouterLink
                  variant="outline"
                  href={`${base}/edit`}>Edit project</UI.RouterLink
                >{/snippet}
            </UI.AdminHeader>
            <UI.NavigationGrid>
              <UI.NavigationCard
                title="Chats"
                description="Conversations attached to this project."
                href={`/app/wsp/${encodeURIComponent(workspace.id)}/ses?${new URLSearchParams({ project: currentRoute.projectID })}`}
              />
              <UI.NavigationCard
                title="Files"
                description="Upload, inspect, and download project files."
                href={`${base}/files`}
              />
              <UI.NavigationCard
                title="Notes"
                description="Read project documents and their revision history."
                href={`${base}/pnt`}
              />
              <UI.NavigationCard
                title="Tasks"
                description="Track project work and workflow status."
                href={`${base}/tasks`}
              />
              <UI.NavigationCard
                title="Records"
                description="Browse structured records and configure their types."
                href={`${base}/records`}
              />
            </UI.NavigationGrid>
          {/if}
        </UI.AdminContent>
      </SidebarPage.Body>
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
