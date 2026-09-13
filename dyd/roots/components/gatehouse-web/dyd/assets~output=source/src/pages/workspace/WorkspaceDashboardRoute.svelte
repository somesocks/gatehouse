<script lang="ts">
  import { Menu } from "@lucide/svelte"
  import { signOut } from "../../app/auth"
  import { createChat, fetchDashboardChats, type Chat, type ChatSearchResponse } from "../../app/chats"
  import { createProject, fetchDashboardProjects, type Project, type ProjectSearchResponse } from "../../app/projects"
  import { useRuntime } from "../../app/runtime.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import WorkspaceFrame from "../../components/WorkspaceFrame.svelte"
  import type { Route } from "../../route"
  import LoginPage from "../login/LoginPage.svelte"
  import WorkspaceDashboardPage from "./WorkspaceDashboardPage.svelte"

  type DashboardRoute = Extract<Route, { kind: "app-home" | "workspace" | "not-found" }>

  const runtime = useRuntime()
  const { access, activity, auth } = runtime
  let mobileMenuOpen = $state(false)
  let sessions = $state<Chat[]>([])
  let projects = $state<Project[]>([])
  let creatingProject = $state(false)
  let error = $state("")
  let generation = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as DashboardRoute)
  const workspace = $derived(currentRoute.kind === "workspace" ? access.state.workspaces.find((candidate) => candidate.id === currentRoute.workspaceID) ?? null : access.state.workspaces[0] ?? null)
  const workspacePath = (workspaceID: string) => `/app/wsp/${encodeURIComponent(workspaceID)}`
  const chatsPath = (workspaceID: string) => `${workspacePath(workspaceID)}/ses`
  const projectsPath = (workspaceID: string) => `${workspacePath(workspaceID)}/prj`
  const isCurrent = (value: number, workspaceID: string, signal: AbortSignal) => value === generation && !signal.aborted && currentRoute.kind === "workspace" && currentRoute.workspaceID === workspaceID

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })

  function activate(route: DashboardRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    mobileMenuOpen = false
    sessions = []
    projects = []
    creatingProject = false
    error = ""
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

  async function loadRoute(route: DashboardRoute, value: number, signal: AbortSignal): Promise<void> {
    const selected = route.kind === "workspace" ? access.state.workspaces.find((candidate) => candidate.id === route.workspaceID) : access.state.workspaces[0]
    if (selected === undefined) {
      if (access.state.workspaces.length > 0 && !signal.aborted && value === generation) runtime.navigate(workspacePath(access.state.workspaces[0].id), true)
      return
    }
    if (route.kind !== "workspace" || selected.id !== route.workspaceID) {
      if (!signal.aborted && value === generation) runtime.navigate(workspacePath(selected.id), true)
      return
    }
    await Promise.all([refreshSessions(value, selected.id, signal), refreshProjects(value, selected.id, signal)])
    if (isCurrent(value, selected.id, signal)) subscribe(value, selected.id, signal)
  }

  async function refreshSessions(value: number, workspaceID: string, signal: AbortSignal): Promise<boolean> {
    try {
      const response = await fetchDashboardChats(workspaceID, signal)
      if (!isCurrent(value, workspaceID, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("dashboard chats unavailable")
      const loaded = await response.json() as ChatSearchResponse
      if (!isCurrent(value, workspaceID, signal)) return false
      sessions = loaded.sessions
      return true
    } catch {
      return false
    }
  }

  async function refreshProjects(value: number, workspaceID: string, signal: AbortSignal): Promise<boolean> {
    try {
      const response = await fetchDashboardProjects(workspaceID, signal)
      if (!isCurrent(value, workspaceID, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("dashboard projects unavailable")
      const loaded = await response.json() as ProjectSearchResponse
      if (!isCurrent(value, workspaceID, signal)) return false
      projects = loaded.projects
      return true
    } catch {
      return false
    }
  }

  function subscribe(value: number, workspaceID: string, routeSignal: AbortSignal): void {
    unsubscribe = activity.subscribe([
      { name: "workspace-dashboard-workspace", topic: workspaceID, events: ["workspace.*", "workspace_grant.*"] },
      { name: "workspace-dashboard-chats", topic: workspaceID, events: ["session.*"] },
      { name: "workspace-dashboard-projects", topic: workspaceID, events: ["project.*"] },
    ], async ({ names, signal }) => {
      if (!isCurrent(value, workspaceID, routeSignal) || signal.aborted) return
      const refreshed = await Promise.all([
        ...(names.has("workspace-dashboard-workspace") ? [runtime.refresh()] : []),
        ...(names.has("workspace-dashboard-chats") ? [refreshSessions(value, workspaceID, routeSignal)] : []),
        ...(names.has("workspace-dashboard-projects") ? [refreshProjects(value, workspaceID, routeSignal)] : []),
      ])
      if (signal.aborted || !isCurrent(value, workspaceID, routeSignal) || refreshed.some((result) => result === false)) throw new Error("dashboard refresh failed")
    })
    void activity.poll()
  }

  async function createNewChat(): Promise<void> {
    const route = currentRoute
    if (route.kind !== "workspace") return
    const value = generation
    const signal = abortController?.signal
    if (signal === undefined) return
    error = ""
    try {
      const response = await createChat(route.workspaceID, signal)
      if (!isCurrent(value, route.workspaceID, signal)) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("chat could not be created")
      const chat = await response.json() as Chat
      if (!isCurrent(value, route.workspaceID, signal)) return
      sessions = [chat, ...sessions.filter((candidate) => candidate.id !== chat.id)].slice(0, 5)
      runtime.navigate(`${chatsPath(route.workspaceID)}/${encodeURIComponent(chat.id)}`)
    } catch {
      if (route.kind === "workspace" && isCurrent(value, route.workspaceID, signal)) error = "A new chat could not be created. Try again."
    }
  }

  async function createNewProject(): Promise<void> {
    if (creatingProject) return
    const route = currentRoute
    if (route.kind !== "workspace") return
    const value = generation
    const signal = abortController?.signal
    if (signal === undefined) return
    creatingProject = true
    error = ""
    try {
      const response = await createProject(route.workspaceID, signal)
      if (!isCurrent(value, route.workspaceID, signal)) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("project could not be created")
      const project = await response.json() as Project
      if (!isCurrent(value, route.workspaceID, signal)) return
      projects = [project, ...projects.filter((candidate) => candidate.id !== project.id)].slice(0, 5)
      runtime.navigate(`${projectsPath(route.workspaceID)}/${encodeURIComponent(project.id)}`)
    } catch {
      if (route.kind === "workspace" && isCurrent(value, route.workspaceID, signal)) error = "A new project could not be created. Try again."
    } finally {
      if (route.kind === "workspace" && isCurrent(value, route.workspaceID, signal)) creatingProject = false
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
  <LoginPage {auth} onAuthenticated={() => void runtime.refresh()} />
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <main class="auth-shell"><section class="status-card"><p class="eyebrow">Gatehouse</p><h1 class="title is-3">No workspace access</h1><p class="subtitle is-6">Ask an administrator to add you to a workspace group.</p>{#if access.state.systemAccess === "available"}<RouterLink class="button is-primary is-light is-fullwidth" href="/app/system">System</RouterLink>{/if}<button class="button is-danger is-light is-fullwidth" type="button" onclick={() => void logout()}>Log out</button></section></main>
{:else}
  <WorkspaceFrame {mobileMenuOpen} brandTheme onMenuClose={() => mobileMenuOpen = false}>
    {#snippet sidebar()}<RouterLink class="brand" href="/app/">Gatehouse</RouterLink><div class="workspace-switcher"><label for="workspace">Workspace</label><div class="select is-fullwidth"><select id="workspace" value={workspace.id} onchange={(event) => runtime.navigate(workspacePath(event.currentTarget.value))}>{#each access.state.workspaces as candidate (candidate.id)}<option value={candidate.id}>{candidate.name ?? candidate.id}</option>{/each}</select></div></div><nav class="sidebar-nav" aria-label="Workspace navigation"><section class="sidebar-section"><ul><li><RouterLink href={chatsPath(workspace.id)}>Chats</RouterLink></li><li><RouterLink href={projectsPath(workspace.id)}>Projects</RouterLink></li><li><RouterLink href={`${workspacePath(workspace.id)}/grp`}>Groups</RouterLink></li></ul></section></nav>{#if access.state.systemAccess === "available"}<div class="sidebar-system-link"><RouterLink href="/app/system" target="_blank" rel="noopener">System</RouterLink></div>{/if}<div class="sidebar-footer"><span>{auth.state.claims?.principal.name ?? "User"}</span><button class="button is-small is-danger is-light" type="button" onclick={() => void logout()}>Log out</button></div>{/snippet}
    {#snippet header()}<button class="mobile-menu-trigger" type="button" aria-label="Open navigation menu" aria-expanded={mobileMenuOpen} onclick={() => mobileMenuOpen = true}><Menu size={20} strokeWidth={2} aria-hidden="true" /></button><h1 class="workspace-breadcrumb"><span class="workspace-breadcrumb-segment"><span>{workspace.name ?? workspace.id}</span></span></h1>{/snippet}
    <WorkspaceDashboardPage {workspace} {sessions} {projects} {creatingProject} {error} onCreateSession={createNewChat} onCreateProject={createNewProject} />
  </WorkspaceFrame>
{/if}
