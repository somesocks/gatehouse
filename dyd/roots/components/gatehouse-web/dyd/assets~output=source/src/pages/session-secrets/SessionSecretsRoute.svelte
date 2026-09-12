<script lang="ts">
  import { Building, Folder, Lock, Menu, MessageSquare } from "@lucide/svelte"
  import { signOut } from "../../app/auth"
  import { fetchChatSession } from "../../app/chat"
  import { useRuntime } from "../../app/runtime.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import SessionNavigation from "../../components/SessionNavigation.svelte"
  import WorkspaceFrame from "../../components/WorkspaceFrame.svelte"
  import type { Route } from "../../route"
  import SessionSecretsPage from "./SessionSecretsPage.svelte"

  type SecretsRoute = Extract<Route, { kind: "session-secrets" | "session-secret-new" | "session-secret" }>
  type Session = { id: string; created_at: string; name?: string; project?: { id: string; name?: string } }
  type Status = "checking" | "ready" | "unavailable"

  const runtime = useRuntime()
  const { access, activity, auth } = runtime
  let mobileMenuOpen = $state(false)
  let session = $state<Session | null>(null)
  let sessionStatus = $state<Status>("checking")
  let secretBreadcrumb = $state<string | null>(null)
  let generation = 0
  let abortController: AbortController | null = null
  let routeSignal = $state<AbortSignal | null>(null)
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as SecretsRoute)
  const workspace = $derived(access.state.workspaces.find((candidate) => candidate.id === currentRoute.workspaceID) ?? null)
  const workspacePath = (workspaceID: string) => `/app/wsp/${encodeURIComponent(workspaceID)}`
  const sessionsPath = (workspaceID: string) => `${workspacePath(workspaceID)}/ses`
  const sessionPath = (workspaceID: string, sessionID: string) => `${sessionsPath(workspaceID)}/${encodeURIComponent(sessionID)}`
  const sessionSecretsPath = (workspaceID: string, sessionID: string) => `${sessionPath(workspaceID, sessionID)}/secrets`
  const projectPath = (workspaceID: string, projectID: string) => `${workspacePath(workspaceID)}/prj/${encodeURIComponent(projectID)}`
  const isCurrent = (value: number, workspaceID: string, sessionID: string, signal: AbortSignal) => value === generation && !signal.aborted && currentRoute.workspaceID === workspaceID && currentRoute.sessionID === sessionID

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })

  function activate(route: SecretsRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    routeSignal = abortController.signal
    unsubscribe?.()
    unsubscribe = undefined
    mobileMenuOpen = false
    session = null
    sessionStatus = "checking"
    secretBreadcrumb = null
    if (auth.state.status === "authenticated" && access.state.workspaceStatus === "ready") void loadRoute(route, value, abortController.signal)
    else if (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking") void runtime.refresh()
    return () => {
      if (value === generation) {
        abortController?.abort()
        unsubscribe?.()
        unsubscribe = undefined
      }
    }
  }

  async function loadRoute(route: SecretsRoute, value: number, signal: AbortSignal): Promise<void> {
    const { workspaceID, sessionID } = route
    if (!access.state.workspaces.some((candidate) => candidate.id === workspaceID)) {
      if (isCurrent(value, workspaceID, sessionID, signal)) runtime.navigate(access.state.workspaces.length === 0 ? "/app/no-access" : workspacePath(access.state.workspaces[0].id), true)
      return
    }
    if (!await loadSession(value, workspaceID, sessionID, signal)) return
    if (!isCurrent(value, workspaceID, sessionID, signal)) return
    subscribe(route, value, signal)
  }

  async function loadSession(value: number, workspaceID: string, sessionID: string, signal: AbortSignal, showLoading = true): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, sessionID, signal)) sessionStatus = "checking"
    try {
      const response = await fetchChatSession(workspaceID, sessionID, signal)
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (response.status === 404) {
        runtime.navigate(sessionsPath(workspaceID), true)
        return false
      }
      if (!response.ok) throw new Error("session unavailable")
      const loaded = await response.json() as Session
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      session = loaded
      sessionStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal)) sessionStatus = "unavailable"
      return false
    }
  }

  function subscribe(route: SecretsRoute, value: number, routeSignal: AbortSignal): void {
    const { workspaceID, sessionID } = route
    unsubscribe = activity.subscribe([{ name: "session-secrets", topic: `${workspaceID}/${sessionID}`, events: ["session.*"] }], async ({ signal }) => {
      if (!isCurrent(value, workspaceID, sessionID, routeSignal) || signal.aborted) return
      if (!await loadSession(value, workspaceID, sessionID, routeSignal, false) || signal.aborted || !isCurrent(value, workspaceID, sessionID, routeSignal)) throw new Error("session secrets refresh failed")
    })
    void activity.poll()
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
  <WorkspaceFrame {mobileMenuOpen} brandTheme onMenuClose={() => mobileMenuOpen = false}>
    {#snippet sidebar()}<RouterLink class="brand" href="/app/">Gatehouse</RouterLink><div class="workspace-switcher"><label for="workspace">Workspace</label><div class="select is-fullwidth"><select id="workspace" value={workspace.id} onchange={(event) => runtime.navigate(workspacePath(event.currentTarget.value))}>{#each access.state.workspaces as candidate (candidate.id)}<option value={candidate.id}>{candidate.name ?? candidate.id}</option>{/each}</select></div></div><nav class="sidebar-nav" aria-label="Workspace navigation"><section class="sidebar-section"><ul><li><RouterLink class="active" href={sessionsPath(workspace.id)}>Chats</RouterLink></li><li><RouterLink href={`${workspacePath(workspace.id)}/prj`}>Projects</RouterLink></li><li><RouterLink href={`${workspacePath(workspace.id)}/grp`}>Groups</RouterLink></li></ul></section></nav>{#if access.state.systemAccess === "available"}<div class="sidebar-system-link"><RouterLink href="/app/system" target="_blank" rel="noopener">System</RouterLink></div>{/if}<div class="sidebar-footer"><span>{auth.state.claims?.principal.name ?? "User"}</span><button class="button is-small is-danger is-light" type="button" onclick={() => void logout()}>Log out</button></div>{/snippet}
    {#snippet header()}<button class="mobile-menu-trigger" type="button" aria-label="Open navigation menu" aria-expanded={mobileMenuOpen} onclick={() => mobileMenuOpen = true}><Menu size={20} strokeWidth={2} aria-hidden="true" /></button><h1 class="workspace-breadcrumb"><RouterLink class="workspace-breadcrumb-segment" href={workspacePath(workspace.id)}><Building size={16} strokeWidth={2} aria-hidden="true" /><span>{workspace.name ?? workspace.id}</span></RouterLink>{#if session !== null}{#if session.project !== undefined}<span class="workspace-breadcrumb-separator" aria-hidden="true">/</span><RouterLink class="workspace-breadcrumb-segment" href={projectPath(workspace.id, session.project.id)}><Folder size={16} strokeWidth={2} aria-hidden="true" /><span>{session.project.name ?? "New Project"}</span></RouterLink>{/if}<span class="workspace-breadcrumb-separator" aria-hidden="true">/</span><RouterLink class="workspace-breadcrumb-segment" href={sessionPath(workspace.id, session.id)}><MessageSquare size={16} strokeWidth={2} aria-hidden="true" /><span>{session.name ?? "New Chat"}</span></RouterLink><span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>{#if currentRoute.kind === "session-secrets"}<span>Secrets</span>{:else}<RouterLink href={sessionSecretsPath(workspace.id, session.id)}>Secrets</RouterLink>{#if secretBreadcrumb !== null}<span class="workspace-breadcrumb-separator" aria-hidden="true">/</span><span class="workspace-breadcrumb-segment"><Lock size={16} strokeWidth={2} aria-hidden="true" />{secretBreadcrumb}</span>{/if}{/if}{/if}</h1>{#if session !== null}<SessionNavigation workspaceID={workspace.id} sessionID={session.id} active="secrets" />{/if}{/snippet}
    {#if sessionStatus === "checking"}<p class="dashboard-empty" aria-busy="true" aria-live="polite">Loading chat...</p>
    {:else if sessionStatus === "unavailable"}<p class="dashboard-empty">This chat could not be loaded.</p><button class="button is-primary" type="button" onclick={() => { const signal = abortController?.signal; if (signal !== undefined) void loadRoute(currentRoute, generation, signal) }}>Try again</button>
    {:else if session !== null && routeSignal !== null}<SessionSecretsPage workspaceID={workspace.id} sessionID={session.id} route={currentRoute} signal={routeSignal} {activity} onAuthenticationLost={() => runtime.requireLogin()} onNavigate={(path, replace) => runtime.navigate(path, replace)} onBreadcrumbChange={(title) => secretBreadcrumb = title} />
    {/if}
  </WorkspaceFrame>
{/if}
