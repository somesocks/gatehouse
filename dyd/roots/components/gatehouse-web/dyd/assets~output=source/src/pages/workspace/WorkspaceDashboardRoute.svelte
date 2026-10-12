<script lang="ts">
  import { RouterLink, StatusPage } from "../../ui"
  import * as UI from "../../ui"
  import * as SidebarPage from "../../ui/sidebar-page"
  import { workspaceNavigation } from "../../app/navigation"
  import { signOut } from "../../app/auth"
  import { useRuntime } from "../../app/runtime.svelte"
  import type { Route } from "../../route"
  import LoginPage from "../login/LoginPage.svelte"
  import WorkspaceDashboardPage from "./WorkspaceDashboardPage.svelte"

  type DashboardRoute = Extract<
    Route,
    { kind: "app-home" | "workspace" | "not-found" }
  >
  const runtime = useRuntime()
  const { access, activity, auth } = runtime
  let generation = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined
  const currentRoute = $derived(runtime.state.route as DashboardRoute)
  const workspace = $derived(
    currentRoute.kind === "workspace"
      ? (access.state.workspaces.find(
          (candidate) => candidate.id === currentRoute.workspaceID,
        ) ?? null)
      : (access.state.workspaces[0] ?? null),
  )
  const workspacePath = (id: string) => `/app/wsp/${encodeURIComponent(id)}`
  const isCurrent = (value: number, workspaceID: string, signal: AbortSignal) =>
    value === generation &&
    !signal.aborted &&
    currentRoute.kind === "workspace" &&
    currentRoute.workspaceID === workspaceID

  $effect(() => activate(currentRoute))

  function activate(route: DashboardRoute): () => void {
    const value = ++generation
    abortController?.abort()
    const controller = new AbortController()
    abortController = controller
    unsubscribe?.()
    unsubscribe = undefined
    if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "ready"
    ) {
      const selected =
        route.kind === "workspace"
          ? access.state.workspaces.find(
              (candidate) => candidate.id === route.workspaceID,
            )
          : access.state.workspaces[0]
      if (!selected) {
        if (access.state.workspaces.length > 0)
          runtime.navigate(workspacePath(access.state.workspaces[0].id), true)
      } else if (
        route.kind !== "workspace" ||
        selected.id !== route.workspaceID
      ) {
        runtime.navigate(workspacePath(selected.id), true)
      } else {
        subscribe(value, selected.id, controller.signal)
      }
    } else if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "checking"
    ) {
      void runtime.refresh()
    }
    return () => {
      controller.abort()
      if (value === generation) {
        unsubscribe?.()
        unsubscribe = undefined
      }
    }
  }

  function subscribe(
    value: number,
    workspaceID: string,
    routeSignal: AbortSignal,
  ) {
    unsubscribe = activity.subscribe(
      [
        {
          name: "workspace-dashboard-workspace",
          topic: workspaceID,
          events: ["workspace.*", "workspace_grant.*"],
        },
      ],
      async ({ signal }) => {
        if (!isCurrent(value, workspaceID, routeSignal) || signal.aborted)
          return
        await runtime.refresh()
      },
    )
    void activity.poll()
  }

  async function logout() {
    try {
      await signOut()
    } finally {
      runtime.requireLogin()
    }
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
  <LoginPage {auth} onAuthenticated={() => void runtime.refresh()} />
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <StatusPage
    eyebrow="Gatehouse"
    title="No workspace access"
    description="Ask an administrator to add you to a workspace group."
  >
    {#if access.state.systemAccess === "available"}<UI.Group
        ><RouterLink variant="default" href="/app/system">System</RouterLink
        ></UI.Group
      >{/if}
    <UI.Group
      ><UI.Button variant="outline" onclick={() => void logout()}
        >Log out</UI.Button
      ></UI.Group
    >
  </StatusPage>
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar
      ><UI.Navigation
        {...workspaceNavigation(runtime, workspace, undefined)}
      /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header>
        <SidebarPage.Toggle />
        <UI.Breadcrumb.Root aria-label="Breadcrumb"
          ><UI.Breadcrumb.List
            ><UI.Breadcrumb.Item aria-current="page"
              ><UI.Breadcrumb.Page
                >{workspace.name ?? "Workspace"}</UI.Breadcrumb.Page
              ></UI.Breadcrumb.Item
            ></UI.Breadcrumb.List
          ></UI.Breadcrumb.Root
        >
      </SidebarPage.Header>
      <SidebarPage.Body><WorkspaceDashboardPage {workspace} /></SidebarPage.Body
      >
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
