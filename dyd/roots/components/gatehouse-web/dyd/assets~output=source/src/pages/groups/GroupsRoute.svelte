<script lang="ts">
  import { Menu } from "@lucide/svelte"
  import { useRuntime } from "../../app/runtime.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import * as SidebarPage from "../../components/sidebar-page"
  import WorkspaceNavigation from "../../components/WorkspaceNavigation.svelte"
  import type { Route } from "../../route"
  import GroupsPage from "./GroupsPage.svelte"

  type GroupsRoute = Extract<Route, { kind: "group-collection" }>

  const runtime = useRuntime()
  const { access, activity, auth } = runtime
  let generation = 0
  let abortController: AbortController | null = null
  let routeSignal = $state<AbortSignal | null>(null)

  const currentRoute = $derived(runtime.state.route as GroupsRoute)
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const workspacePath = (workspaceID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}`

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })

  function activate(route: GroupsRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    routeSignal = abortController.signal
    if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "ready"
    )
      loadRoute(route, value, abortController.signal)
    else if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "checking"
    )
      void runtime.refresh()
    return () => {
      if (value === generation) abortController?.abort()
    }
  }

  function loadRoute(
    route: GroupsRoute,
    value: number,
    signal: AbortSignal,
  ): void {
    if (
      access.state.workspaces.some(
        (candidate) => candidate.id === route.workspaceID,
      )
    )
      return
    if (
      value === generation &&
      !signal.aborted &&
      currentRoute.workspaceID === route.workspaceID
    ) {
      runtime.navigate(
        access.state.workspaces.length === 0
          ? "/app/no-access"
          : workspacePath(access.state.workspaces[0].id),
        true,
      )
    }
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <main class="status-page" aria-busy="true" aria-live="polite">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <div class="loading-mark" aria-hidden="true"></div>
      <p>
        {auth.state.status === "checking"
          ? "Checking your session."
          : "Loading your workspaces."}
      </p>
    </section>
  </main>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <main class="status-page">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">Connection unavailable</h1>
      <p class="subtitle is-6">Gatehouse could not load your account.</p>
      <button
        class="button is-primary"
        type="button"
        onclick={() => void runtime.refresh()}>Try again</button
      >
    </section>
  </main>
{:else if auth.state.status !== "authenticated"}
  <main class="status-page">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">Sign in required</h1>
      <button
        class="button is-primary"
        type="button"
        onclick={() => runtime.requireLogin()}>Sign in</button
      >
    </section>
  </main>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <main class="status-page">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">No workspace access</h1>
      <p class="subtitle is-6">
        Ask an administrator to add you to a workspace group.
      </p>
      <button
        class="button is-primary"
        type="button"
        onclick={() => runtime.navigate("/app/no-access", true)}
        >Continue</button
      >
    </section>
  </main>
{:else}
  <SidebarPage.Root
    ><SidebarPage.Sidebar
      ><WorkspaceNavigation {workspace} active="groups" /></SidebarPage.Sidebar
    ><SidebarPage.Page
      ><SidebarPage.Header
        ><SidebarPage.Toggle
          ><button
            class="mobile-menu-trigger"
            type="button"
            aria-label="Open navigation menu"
            ><Menu size={20} strokeWidth={2} aria-hidden="true" /></button
          ></SidebarPage.Toggle
        >
        <h1 class="brand-workspace-breadcrumb">
          <RouterLink
            class="brand-workspace-breadcrumb-segment"
            href={workspacePath(workspace.id)}
            ><span>{workspace.name ?? workspace.id}</span></RouterLink
          ><span class="brand-workspace-breadcrumb-separator" aria-hidden="true"
            >/</span
          ><span>Groups</span>
        </h1></SidebarPage.Header
      ><SidebarPage.Body
        >{#if routeSignal !== null}<GroupsPage
            {workspace}
            signal={routeSignal}
            {activity}
            onAuthenticationLost={() => runtime.requireLogin()}
          />{/if}</SidebarPage.Body
      ></SidebarPage.Page
    ></SidebarPage.Root
  >
{/if}
