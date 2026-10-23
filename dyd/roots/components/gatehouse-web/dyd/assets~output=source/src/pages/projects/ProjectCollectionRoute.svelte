<script lang="ts">
  import { untrack } from "svelte"
  import { RouterLink, StatusPage } from "../../ui"
  import { workspaceNavigation } from "../../app/navigation"
  import * as UI from "../../ui"
  import { useRuntime } from "../../app/runtime.svelte"
  import * as SidebarPage from "../../ui/sidebar-page"

  import type { Route } from "../../route"
  import ProjectCollectionPage from "./ProjectCollectionPage.svelte"

  type ProjectCollectionRoute = Extract<Route, { kind: "project-collection" }>

  const runtime = useRuntime()
  const { access, auth } = runtime
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
  const isCurrent = (value: number, workspaceID: string, signal: AbortSignal) =>
    value === generation &&
    !signal.aborted &&
    currentRoute.workspaceID === workspaceID

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

  function activate(route: ProjectCollectionRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    routeSignal = abortController.signal
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

</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <StatusPage eyebrow="Gatehouse" busy live="polite">
    {#snippet children()}
      <UI.Text as="p">
        {auth.state.status === "checking"
          ? "Checking your session."
          : "Loading your workspaces."}
      </UI.Text>
    {/snippet}
  </StatusPage>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage
    eyebrow="Gatehouse"
    title="Connection unavailable"
    description="Gatehouse could not load your account."
  >
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => void runtime.refresh()}>Try again</UI.Button
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else if auth.state.status !== "authenticated"}
  <StatusPage eyebrow="Gatehouse" title="Sign in required">
    {#snippet children()}<UI.Group
      ><UI.RouterLink
        variant="default"
        href={runtime.loginHref} replace>Sign in</UI.RouterLink
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <StatusPage
    eyebrow="Gatehouse"
    title="No workspace access"
    description="Ask an administrator to add you to a workspace group."
  >
    {#snippet children()}<UI.Group
      ><UI.RouterLink
        variant="default"
        href="/app/no-access" replace
        >Continue</UI.RouterLink
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else}
  <SidebarPage.Root
    ><SidebarPage.Sidebar
      ><UI.Navigation
        {...workspaceNavigation(runtime, workspace, "projects")}
      /></SidebarPage.Sidebar
    ><SidebarPage.Page
      ><SidebarPage.Header
        ><SidebarPage.Toggle />
        <UI.Breadcrumb.Root aria-label="Breadcrumb"
          ><UI.Breadcrumb.List
            ><UI.Breadcrumb.Item
              ><RouterLink href={workspacePath(workspace.id)}
                >{workspace.name ?? "Workspace"}</RouterLink
              ></UI.Breadcrumb.Item
            ><UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
              ><UI.Breadcrumb.Page>Projects</UI.Breadcrumb.Page
              ></UI.Breadcrumb.Item
            ></UI.Breadcrumb.List
          ></UI.Breadcrumb.Root
        ></SidebarPage.Header
      ><SidebarPage.Body
        >{#if routeSignal !== null}<ProjectCollectionPage
            {workspace}
            search={currentRoute.search}
            signal={routeSignal}
            onAuthenticationLost={() => runtime.requireLogin()}
            onNavigate={runtime.navigate}
            activity={runtime.activity}
          />{/if}</SidebarPage.Body
      ></SidebarPage.Page
    ></SidebarPage.Root
  >
{/if}
