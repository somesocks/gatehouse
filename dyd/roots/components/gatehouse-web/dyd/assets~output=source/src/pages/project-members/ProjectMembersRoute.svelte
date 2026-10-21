<script lang="ts">
  import { untrack } from "svelte"
  import * as UI from "../../ui"
  import { workspaceNavigation } from "../../app/navigation"
  import { useRuntime } from "../../app/runtime.svelte"
  import { fetchProject, type Project } from "../../app/projects"
  import { createLoadState } from "../../utils/load-state.svelte"
  import type { Route } from "../../route"
  import ProjectMembersPage from "./ProjectMembersPage.svelte"

  const runtime = useRuntime()
  const resource = createLoadState()
  const route = $derived(
    runtime.state.route as Extract<Route, { kind: "project-members" }>,
  )
  const workspace = $derived(
    runtime.access.state.workspaces.find((item) => item.id === route.workspaceID),
  )
  const listPath = $derived(`/app/wsp/${encodeURIComponent(route.workspaceID)}/prj`)
  const projectPath = $derived(`${listPath}/${encodeURIComponent(route.projectID)}`)
  let project = $state<Project | null>(null)
  let controller: AbortController | undefined

  const activation = $derived(JSON.stringify([
    route,
    runtime.auth.state.status,
    runtime.auth.state.claims?.principal.ref.id,
    runtime.access.state.workspaceStatus,
    workspace !== undefined,
  ]))
  $effect(() => {
    activation
    return untrack(() => {
      const selected = route
      controller?.abort()
      const own = new AbortController()
      controller = own
      resource.reset()
      project = null
      let unsubscribe: (() => void) | undefined
      if (
        runtime.auth.state.status === "authenticated" &&
        runtime.access.state.workspaceStatus === "ready" && workspace
      ) {
        void load(selected, own.signal)
        unsubscribe = runtime.activity.subscribe([
          {
            name: "project-members-project",
            topic: `${selected.workspaceID}/${selected.projectID}`,
            events: ["project.*"],
          },
        ], async ({ signal }) => {
          if (own.signal.aborted || signal.aborted) return
          if (!(await load(selected, AbortSignal.any([own.signal, signal]))))
            throw new Error("Project could not be refreshed.")
        })
        void runtime.activity.poll()
      }
      return () => {
        own.abort()
        resource.stop()
        unsubscribe?.()
      }
    })
  })

  async function load(selected: Extract<Route, { kind: "project-members" }>, signal: AbortSignal) {
    const request = resource.begin(signal)
    try {
      const response = await fetchProject(selected.workspaceID, selected.projectID, request.signal)
      if (!request.current()) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (response.status === 404) {
        runtime.navigate(`/app/wsp/${encodeURIComponent(selected.workspaceID)}/prj`, true)
        return false
      }
      if (!response.ok) throw new Error("Project could not be loaded.")
      const loaded = await response.json() as Project
      return request.commit(() => { project = loaded })
    } catch (error) {
      request.fail(error)
      return false
    } finally {
      request.finish()
    }
  }
</script>

{#if runtime.auth.state.status === "checking" || (runtime.auth.state.status === "authenticated" && runtime.access.state.workspaceStatus === "checking")}
  <UI.StatusPage title="Loading workspace" description="Preparing project members." busy />
{:else if runtime.auth.state.status === "unavailable" || runtime.access.state.workspaceStatus === "unavailable"}
  <UI.StatusPage title="Connection unavailable">
    <UI.Button onclick={() => void runtime.refresh()}>Try again</UI.Button>
  </UI.StatusPage>
{:else if runtime.auth.state.status !== "authenticated"}
  <UI.StatusPage title="Sign in required">
    <UI.Button onclick={() => runtime.requireLogin()}>Sign in</UI.Button>
  </UI.StatusPage>
{:else if !workspace}
  <UI.StatusPage title="No workspace access" description="Ask an administrator to grant you workspace access.">
    <UI.Button onclick={() => runtime.navigate("/app/no-access", true)}>Continue</UI.Button>
  </UI.StatusPage>
{:else}
  <UI.AppFrame
    navigation={workspaceNavigation(runtime, workspace, "projects")}
    breadcrumbs={[
      { label: workspace.name ?? "Workspace", href: `/app/wsp/${encodeURIComponent(workspace.id)}` },
      { label: "Projects", href: listPath },
      { label: project?.name ?? "Project", href: projectPath },
      { label: "Members" },
    ]}
  >
    {#if resource.reloading}<UI.Text role="status">Refreshing project…</UI.Text>{/if}
    {#if resource.state.error}
      <UI.Notice variant="destructive" role="alert">
        {resource.state.error}
        <UI.Button variant="outline" onclick={() => controller && void load(route, controller.signal)}>Retry</UI.Button>
      </UI.Notice>
    {/if}
    {#if project}
      {#key `${route.workspaceID}/${route.projectID}`}
        <ProjectMembersPage
          workspaceID={route.workspaceID}
          projectID={route.projectID}
          onAccessLost={() => runtime.navigate(listPath, true)}
        />
      {/key}
    {:else}
      <UI.AdminContent mode="detail">
        <UI.Text role="status">{resource.loading ? "Loading project…" : "Project could not be loaded."}</UI.Text>
      </UI.AdminContent>
    {/if}
  </UI.AppFrame>
{/if}
