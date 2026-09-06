<script lang="ts">
  import { Building, Menu } from "@lucide/svelte"
  import { signOut } from "../../app/auth"
  import { useRuntime } from "../../app/runtime.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import WorkspaceFrame from "../../components/WorkspaceFrame.svelte"
  import type { Route } from "../../route"
  import GroupsPage from "./GroupsPage.svelte"

  type GroupsRoute = Extract<Route, { kind: "group-collection" }>

  const runtime = useRuntime()
  const { access, activity, auth } = runtime
  let mobileMenuOpen = $state(false)
  let generation = 0
  let abortController: AbortController | null = null
  let routeSignal = $state<AbortSignal | null>(null)

  const currentRoute = $derived(runtime.state.route as GroupsRoute)
  const workspace = $derived(access.state.workspaces.find((candidate) => candidate.id === currentRoute.workspaceID) ?? null)
  const workspacePath = (workspaceID: string) => `/app/wsp/${encodeURIComponent(workspaceID)}`

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })

  function activate(route: GroupsRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    routeSignal = abortController.signal
    mobileMenuOpen = false
    if (auth.state.status === "authenticated" && access.state.workspaceStatus === "ready") loadRoute(route, value, abortController.signal)
    else if (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking") void runtime.refresh()
    return () => {
      if (value === generation) abortController?.abort()
    }
  }

  function loadRoute(route: GroupsRoute, value: number, signal: AbortSignal): void {
    if (access.state.workspaces.some((candidate) => candidate.id === route.workspaceID)) return
    if (value === generation && !signal.aborted && currentRoute.workspaceID === route.workspaceID) {
      runtime.navigate(access.state.workspaces.length === 0 ? "/app/no-access" : workspacePath(access.state.workspaces[0].id), true)
    }
  }

  async function logout(): Promise<void> {
    try {
      await signOut()
    } finally {
      runtime.requireLogin()
    }
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <main class="auth-shell" aria-busy="true" aria-live="polite"><section class="status-card"><p class="eyebrow">Gatehouse</p><div class="loading-mark" aria-hidden="true"></div><p>{auth.state.status === "checking" ? "Checking your session." : "Loading your workspaces."}</p></section></main>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <main class="auth-shell"><section class="status-card"><p class="eyebrow">Gatehouse</p><h1 class="title is-3">Connection unavailable</h1><p class="subtitle is-6">Gatehouse could not load your account.</p><button class="button is-primary" type="button" onclick={() => void runtime.refresh()}>Try again</button></section></main>
{:else if auth.state.status !== "authenticated"}
  <main class="auth-shell"><section class="status-card"><p class="eyebrow">Gatehouse</p><h1 class="title is-3">Sign in required</h1><button class="button is-primary" type="button" onclick={() => runtime.requireLogin()}>Sign in</button></section></main>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <main class="auth-shell"><section class="status-card"><p class="eyebrow">Gatehouse</p><h1 class="title is-3">No workspace access</h1><p class="subtitle is-6">Ask an administrator to add you to a workspace group.</p><button class="button is-primary" type="button" onclick={() => runtime.navigate("/app/no-access", true)}>Continue</button></section></main>
{:else}
  <WorkspaceFrame {mobileMenuOpen} onMenuClose={() => mobileMenuOpen = false}>
    {#snippet sidebar()}<RouterLink class="brand" href="/app/">Gatehouse</RouterLink><div class="workspace-switcher"><label for="workspace">Workspace</label><div class="select is-fullwidth"><select id="workspace" value={workspace.id} onchange={(event) => runtime.navigate(workspacePath(event.currentTarget.value))}>{#each access.state.workspaces as candidate (candidate.id)}<option value={candidate.id}>{candidate.name ?? candidate.id}</option>{/each}</select></div></div><nav class="sidebar-nav" aria-label="Workspace navigation"><section class="sidebar-section"><ul><li><RouterLink href={`${workspacePath(workspace.id)}/ses`}>Chats</RouterLink></li><li><RouterLink href={`${workspacePath(workspace.id)}/prj`}>Projects</RouterLink></li><li><RouterLink class="active" href={`${workspacePath(workspace.id)}/grp`}>Groups</RouterLink></li></ul></section></nav>{#if access.state.systemAccess === "available"}<div class="sidebar-system-link"><RouterLink href="/app/system" target="_blank" rel="noopener">System</RouterLink></div>{/if}<div class="sidebar-footer"><span>{auth.state.claims?.principal.name ?? "User"}</span><button class="button is-small is-danger is-light" type="button" onclick={() => void logout()}>Log out</button></div>{/snippet}
    {#snippet header()}<button class="mobile-menu-trigger" type="button" aria-label="Open navigation menu" aria-expanded={mobileMenuOpen} onclick={() => mobileMenuOpen = true}><Menu size={20} strokeWidth={2} aria-hidden="true" /></button><h1 class="workspace-breadcrumb"><RouterLink class="workspace-breadcrumb-segment" href={workspacePath(workspace.id)}><Building size={16} strokeWidth={2} aria-hidden="true" /><span>{workspace.name ?? workspace.id}</span></RouterLink><span class="workspace-breadcrumb-separator" aria-hidden="true">/</span><span>Groups</span></h1>{/snippet}
    {#if routeSignal !== null}<GroupsPage {workspace} signal={routeSignal} {activity} onAuthenticationLost={() => runtime.requireLogin()} />{/if}
  </WorkspaceFrame>
{/if}
