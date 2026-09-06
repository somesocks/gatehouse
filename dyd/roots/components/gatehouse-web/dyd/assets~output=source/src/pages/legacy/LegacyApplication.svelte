<script lang="ts">
  import { Building, Folder, Menu, MessageSquare } from "@lucide/svelte"
  import type { ActivitySelector } from "../../utils/activity-poller"
  import type { Workspace } from "../../app/access"
  import { signOut } from "../../app/auth"
  import { useRuntime } from "../../app/runtime.svelte"
  import ChatCollectionPage from "../chats/ChatCollectionPage.svelte"
  import GroupsPage from "../groups/GroupsPage.svelte"
  import LoginPage from "../login/LoginPage.svelte"
  import ProjectCollectionPage from "../projects/ProjectCollectionPage.svelte"
  import WorkspaceDashboardPage from "../workspace/WorkspaceDashboardPage.svelte"
  import SystemPage from "../system/SystemPage.svelte"
  import { routeProjectID, routeSessionID, routeWorkspaceID } from "../../route"
  import type { Route } from "../../route"

  type Session = {
    id: string
    created_at: string
    name?: string
    project?: Project
  }

  type Project = {
    id: string
    created_at: string
    name?: string
    description?: string
  }

  type SessionSearchResponse = {
    sessions: Session[]
    next_cursor?: string
  }

  type ProjectSearchResponse = {
    projects: Project[]
    next_cursor?: string
  }

  type WorkspaceContentStatus = "checking" | "ready" | "unavailable"
  type ActivityProjection = { stop: () => void }

  const runtime = useRuntime()
  const { auth, access, activity } = runtime
  let workspaceContentStatus = $state<WorkspaceContentStatus>("checking")
  let route = $derived(runtime.state.route)
  let activeWorkspace = $state<Workspace | null>(null)
  let latestProjects = $state<Project[]>([])
  let latestSessions = $state<Session[]>([])
  let activeSession = $state<Session | null>(null)
  let activeProject = $state<Project | null>(null)
  let creatingProject = $state(false)
  let messageError = $state("")
  let mobileMenuOpen = $state(false)
  let activeProjection: ActivityProjection | null = null
  let routeGeneration = 0
  let routeAbortController: AbortController | null = null
  $effect(() => {
    const next = route
    routeAbortController?.abort()
    routeAbortController = new AbortController()
    void activateRoute(++routeGeneration, routeAbortController.signal)
  })
  $effect(() => () => {
    routeAbortController?.abort()
    stopActivityPolling()
  })

  function isLoginPath() {
    return route.kind === "login"
  }

  function isSystemRoute() {
    return route.kind === "system" || route.kind === "system-grants" || route.kind === "system-principals"
  }

  function nextPath() {
    const next = route.kind === "login" ? route.next : null
    if (next === null) {
      return "/app/"
    }
    let destination: URL
    try {
      destination = new URL(next, window.location.origin)
    } catch {
      return "/app/"
    }
    if (destination.origin !== window.location.origin || !destination.pathname.startsWith("/app/") || destination.pathname === "/app/login" || destination.pathname === "/app/login/") {
      return "/app/"
    }
    return destination.pathname + destination.search + destination.hash
  }

  function workspacePath(workspace: Workspace) {
    return `/app/wsp/${encodeURIComponent(workspace.id)}`
  }

  function sessionsPath(workspace: Workspace) {
    return `${workspacePath(workspace)}/ses`
  }

  function sessionPath(workspace: Workspace, session: Session) {
    return `${sessionsPath(workspace)}/${encodeURIComponent(session.id)}`
  }

  function sessionNotesPath(workspace: Workspace, session: Session) {
    return `${sessionPath(workspace, session)}/notes`
  }

  function sessionSecretsPath(workspace: Workspace, session: Session) {
    return `${sessionPath(workspace, session)}/secrets`
  }

  function projectsPath(workspace: Workspace) {
    return `${workspacePath(workspace)}/prj`
  }

  function projectPath(workspace: Workspace, project: Project) {
    return `${projectsPath(workspace)}/${encodeURIComponent(project.id)}`
  }

  function groupsPath(workspace: Workspace) {
    return `${workspacePath(workspace)}/grp`
  }

  function isChatCollection() {
    return route.kind === "session-collection"
  }

  function isProjectCollection() {
    return route.kind === "project-collection"
  }

  function isGroupCollection() {
    return route.kind === "group-collection"
  }

  function navigate(path: string, replace = false) {
    runtime.navigate(path, replace)
  }

  function isCurrentRoute(generation: number) {
    return generation === routeGeneration
  }

  function signInRequired() {
    routeAbortController?.abort()
    routeGeneration += 1
    stopActivityPolling()
    activeWorkspace = null
    latestProjects = []
    latestSessions = []
    activeSession = null
    activeProject = null
    runtime.requireLogin()
  }

  async function refreshRuntime(): Promise<boolean> {
    await runtime.refresh()
    const workspaces = access.state.workspaces
    if (access.state.workspaceStatus === "empty") {
      activeWorkspace = null
      latestProjects = []
      latestSessions = []
      if (route.kind !== "no-access" && !isSystemRoute()) {
        navigate("/app/no-access", true)
      }
      return true
    }
    if (access.state.workspaceStatus !== "ready") {
      return false
    }
    const currentWorkspaceID = activeWorkspace?.id ?? routeWorkspaceID(route)
    const currentWorkspace = currentWorkspaceID === null ? undefined : workspaces.find((workspace) => workspace.id === currentWorkspaceID)
    if (currentWorkspaceID !== null && currentWorkspace === undefined) {
      activeWorkspace = null
      navigate(workspacePath(workspaces[0]), true)
      return true
    }
    if (currentWorkspace !== undefined) {
      activeWorkspace = currentWorkspace
    }
    return access.state.workspaceStatus === "ready"
  }

  async function activateRoute(generation: number, signal: AbortSignal) {
    if (auth.state.status === "checking") {
      void runtime.refresh()
      return
    }
    if (auth.state.status !== "authenticated") {
      return
    }
    if (isSystemRoute()) {
      return
    }
    if (access.state.workspaceStatus === "checking") {
      void runtime.refresh()
      return
    }
    if (access.state.workspaceStatus !== "ready") {
      return
    }
    if (route.kind === "login") {
      navigate(nextPath(), true)
      return
    }
    const requestedWorkspaceID = routeWorkspaceID(route)
    const workspace = access.state.workspaces.find((candidate) => candidate.id === requestedWorkspaceID) ?? access.state.workspaces[0]
    if (requestedWorkspaceID !== workspace.id) {
      navigate(workspacePath(workspace), true)
      return
    }
    if (!await activateWorkspace(workspace, generation, signal) || !isCurrentRoute(generation) || signal.aborted) {
      return
    }
    const sessionID = routeSessionID(route)
    if (sessionID !== null) {
      const session = activeSession?.id === sessionID ? activeSession : latestSessions.find((candidate) => candidate.id === sessionID) ?? await loadSession(workspace, sessionID)
      if (!isCurrentRoute(generation)) {
        return
      }
      if (session === null || session === undefined) {
        navigate(sessionsPath(workspace), true)
        return
      }
      await activateSession(session, generation)
       await initializeRouteProjection(generation, () => loadInitialSessionProjection())
      return
    }
    const projectID = routeProjectID(route)
    if (projectID !== null) {
      const project = activeProject?.id === projectID ? activeProject : latestProjects.find((candidate) => candidate.id === projectID) ?? await loadProject(workspace, projectID)
      if (!isCurrentRoute(generation)) {
        return
      }
      if (project === null || project === undefined) {
        navigate(projectsPath(workspace), true)
        return
      }
      await activateProject(project, generation)
      await initializeRouteProjection(generation, () => loadInitialProjectProjection(workspace, project))
      return
    }
    activateWorkspacePage()
    await initializeRouteProjection(generation, () => loadInitialWorkspaceProjection(workspace))
  }

  async function initializeRouteProjection(generation: number, load: () => Promise<unknown>) {
    if (!isCurrentRoute(generation)) {
      return
    }
    configureActivityPolling()
    if (isCurrentRoute(generation)) {
      await load()
    }
  }

  async function loadInitialWorkspaceProjection(workspace: Workspace) {
    if (!isChatCollection() && !isProjectCollection() && !isGroupCollection()) {
      await Promise.all([refreshWorkspaceSessions(workspace), refreshWorkspaceProjects(workspace)])
    }
  }

  async function loadInitialSessionProjection(): Promise<void> {}

  async function loadInitialProjectProjection(workspace: Workspace, project: Project) {}

  async function activateWorkspace(workspace: Workspace, generation: number, signal: AbortSignal) {
    if (activeWorkspace?.id === workspace.id) {
      activeWorkspace = workspace
      return true
    }
    mobileMenuOpen = false
    stopActivityPolling()
    activeWorkspace = workspace
    latestProjects = []
    latestSessions = []
    activeSession = null
    activeProject = null
    workspaceContentStatus = "checking"
    return !signal.aborted && isCurrentRoute(generation)
  }

  function activateWorkspacePage() {
    stopActivityPolling()
    activeSession = null
    activeProject = null
  }

  async function activateSession(session: Session, generation: number) {
    if (activeWorkspace === null) {
      return
    }
    mobileMenuOpen = false
    const changedSession = activeSession?.id !== session.id
    stopActivityPolling()
    activeSession = session
    activeProject = session.project ?? null
    if (changedSession) {
      messageError = ""
    }
  }

  async function activateProject(project: Project, generation: number) {
    if (activeWorkspace === null) {
      return
    }
    mobileMenuOpen = false
    stopActivityPolling()
    activeSession = null
    activeProject = project
  }

  async function loadSession(workspace: Workspace, id: string) {
    const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions/${encodeURIComponent(id)}`, { credentials: "same-origin" })
    if (response.status === 401) {
      signInRequired()
      return null
    }
    if (response.status === 404) {
      return null
    }
    if (!response.ok) {
      throw new Error("session could not be loaded")
    }
    return (await response.json()) as Session
  }

  async function loadProject(workspace: Workspace, id: string) {
    const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects/${encodeURIComponent(id)}`, { credentials: "same-origin" })
    if (response.status === 401) {
      signInRequired()
      return null
    }
    if (response.status === 404) {
      return null
    }
    if (!response.ok) {
      throw new Error("project could not be loaded")
    }
    return (await response.json()) as Project
  }

  function isActiveProjection(projection: ActivityProjection) {
    return activeProjection === projection
  }

  function stopActivityPolling() {
    disposeActiveProjection()
  }

  function disposeActiveProjection() {
    const projection = activeProjection
    activeProjection = null
    projection?.stop()
  }

  async function refreshWorkspaceSessions(workspace: Workspace, projection?: ActivityProjection) {
    const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions?limit=5`, { credentials: "same-origin" })
    if (response.status === 401) {
      signInRequired()
      return false
    }
    if (!response.ok) {
      throw new Error("sessions could not be refreshed")
    }
    const loaded = (await response.json()) as SessionSearchResponse
    if ((projection !== undefined && !isActiveProjection(projection)) || activeWorkspace?.id !== workspace.id) {
      return false
    }
    latestSessions = loaded.sessions
    if (activeSession !== null) {
      activeSession = loaded.sessions.find((session) => session.id === activeSession?.id) ?? activeSession
    }
    return true
  }

  async function refreshWorkspaceProjects(workspace: Workspace, projection?: ActivityProjection) {
    const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects?limit=5`, { credentials: "same-origin" })
    if (response.status === 401) {
      signInRequired()
      return false
    }
    if (!response.ok) {
      throw new Error("projects could not be refreshed")
    }
    const loaded = (await response.json()) as ProjectSearchResponse
    if ((projection !== undefined && !isActiveProjection(projection)) || activeWorkspace?.id !== workspace.id) {
      return false
    }
    latestProjects = loaded.projects
    if (activeProject !== null) {
      activeProject = loaded.projects.find((project) => project.id === activeProject?.id) ?? activeProject
    }
    return true
  }

  function configureActivityPolling() {
    disposeActiveProjection()
    if (activeWorkspace === null) {
      return
    }
    const workspace = activeWorkspace
    const session = activeSession
    const overview = session === null && activeProject === null
    const workspaceSelector: ActivitySelector = {
      name: "workspace",
      topic: workspace.id,
      events: ["workspace.*", "workspace_grant.*"],
    }
    const agentSelector = session === null ? undefined : {
      name: "agent",
      topic: workspace.id,
      events: ["workspace_agent.*"],
    } satisfies ActivitySelector
    const sessionSelector = session === null ? (overview ? {
      name: "session",
      topic: workspace.id,
      events: ["session.*"],
    } satisfies ActivitySelector : undefined) : {
      name: "session",
      topic: `${workspace.id}/${session.id}`,
      events: ["session.*", "session_event.*", "session_file.*", "session_secret.*"],
    } satisfies ActivitySelector
    const projectID = session?.project?.id ?? activeProject?.id
    const projectSelector = projectID === undefined ? (overview ? {
      name: "project",
      topic: workspace.id,
      events: ["project.*"],
    } satisfies ActivitySelector : undefined) : {
      name: "project",
      topic: `${workspace.id}/${projectID}`,
      events: ["project.*"],
    } satisfies ActivitySelector
    const activitySelectors = [workspaceSelector, agentSelector, sessionSelector, projectSelector].filter((selector): selector is ActivitySelector => selector !== undefined)
    let unsubscribe: (() => void) | undefined
    const projection: ActivityProjection = { stop: () => unsubscribe?.() }
    activeProjection = projection
    unsubscribe = activity.subscribe(activitySelectors, async ({ names, signal }) => {
      if (signal.aborted || !isActiveProjection(projection)) {
        return
      }

      const workspaceChanged = names.has(workspaceSelector.name)
      let refreshed = (await Promise.all([
        ...(workspaceChanged ? [refreshRuntime()] : []),
        ...(sessionSelector !== undefined ? [refreshWorkspaceSessions(workspace, projection)] : []),
        ...(projectSelector !== undefined ? [refreshWorkspaceProjects(workspace, projection)] : []),
      ])).every(Boolean)
      if (!isActiveProjection(projection) || signal.aborted) {
        throw new Error("activity projection refresh failed")
      }
      if (!refreshed || signal.aborted || !isActiveProjection(projection)) {
        throw new Error("activity projection refresh failed")
      }
    })
  }

  async function createSession(project?: Project) {
    if (activeWorkspace === null) {
      return
    }
    mobileMenuOpen = false
    messageError = ""
    try {
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(activeWorkspace.id)}/sessions`, {
        method: "POST",
        credentials: "same-origin",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(project === undefined ? {} : { project: project.id }),
      })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("session could not be created")
      }
      const session = (await response.json()) as Session
      latestSessions = [session, ...latestSessions].slice(0, 5)
      navigate(sessionPath(activeWorkspace, session))
    } catch {
      messageError = "A new chat could not be created. Try again."
    }
  }

  async function createProject() {
    if (activeWorkspace === null) {
      return
    }
    creatingProject = true
    messageError = ""
    try {
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(activeWorkspace.id)}/projects`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({}),
      })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("project could not be created")
      }
      const project = (await response.json()) as Project
      latestProjects = [project, ...latestProjects].slice(0, 5)
      navigate(projectPath(activeWorkspace, project))
    } catch {
      messageError = "A new project could not be created. Try again."
    } finally {
      creatingProject = false
    }
  }

  async function logout() {
    try {
      await signOut()
    } finally {
      signInRequired()
    }
  }
</script>

<svelte:head>
  <meta name="description" content="Gatehouse hosted chat" />
  <title>Gatehouse</title>
</svelte:head>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <main class="auth-shell" aria-busy="true" aria-live="polite">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <div class="loading-mark" aria-hidden="true"></div>
      <p>{auth.state.status === "checking" ? "Checking your session." : "Loading your workspaces."}</p>
    </section>
  </main>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <main class="auth-shell">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">Connection unavailable</h1>
      <p class="subtitle is-6">Gatehouse could not load your account.</p>
       <button class="button is-primary" type="button" onclick={() => void runtime.refresh()}>Try again</button>
    </section>
  </main>
{:else if auth.state.status === "anonymous"}
  <LoginPage {auth} onAuthenticated={() => void runtime.refresh()} />
{:else if isSystemRoute()}
  <SystemPage
    route={route}
    systemAccess={access.state.systemAccess}
    {activity}
    principalID={() => auth.state.claims?.principal.ref.id}
    principalName={auth.state.claims?.principal.name ?? "User"}
    onAuthenticationLost={signInRequired}
    onSystemAccessChange={access.setSystemAccess}
    onNavigate={navigate}
    onLogout={() => void logout()}
    bind:mobileMenuOpen
  />
{:else if access.state.workspaceStatus === "empty"}
  <main class="auth-shell">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">No workspace access</h1>
      <p class="subtitle is-6">Ask an administrator to add {auth.state.claims?.principal.name ?? "User"} to a workspace group.</p>
      {#if access.state.systemAccess === "available"}<button class="button is-primary is-light is-fullwidth" type="button" onclick={() => navigate("/app/system")}>System</button>{/if}
      <button class="button is-danger is-light is-fullwidth" type="button" onclick={() => void logout()}>Log out</button>
    </section>
  </main>
{:else}
  <div class="app-shell">
    {#if mobileMenuOpen}
      <button class="mobile-menu-backdrop" type="button" aria-label="Close navigation menu" onclick={() => mobileMenuOpen = false}></button>
    {/if}
    <aside class:mobile-menu-open={mobileMenuOpen} class="sidebar">
      <a class="brand" href="/app/">Gatehouse</a>

      <div class="workspace-switcher">
        <label for="workspace">Workspace</label>
        <div class="select is-fullwidth">
          <select id="workspace" value={activeWorkspace?.id ?? ""} onchange={(event) => {
            const target = event.currentTarget as HTMLSelectElement
            const workspace = access.state.workspaces.find((candidate) => candidate.id === target.value)
            if (workspace !== undefined) {
              navigate(workspacePath(workspace))
            }
          }}>
            {#each access.state.workspaces as workspace}
              <option value={workspace.id}>{workspace.name ?? workspace.id}</option>
            {/each}
          </select>
        </div>
      </div>

      <nav class="sidebar-nav" aria-label="Workspace navigation">
        <section class="sidebar-section">
          <ul>
            <li><a class:active={isChatCollection()} href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionsPath(activeWorkspace)) } }}>Chats</a></li>
            <li><a class:active={isProjectCollection()} href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/prj`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(projectsPath(activeWorkspace)) } }}>Projects</a></li>
            <li><a class:active={isGroupCollection()} href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/grp`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(groupsPath(activeWorkspace)) } }}>Groups</a></li>
          </ul>
        </section>
      </nav>

      {#if access.state.systemAccess === "available"}
        <div class="sidebar-system-link">
          <a href="/app/system" target="_blank" rel="noopener">System</a>
        </div>
      {/if}
      <div class="sidebar-footer">
        <span>{auth.state.claims?.principal.name ?? "User"}</span>
        <button class="button is-small is-danger is-light" type="button" onclick={() => void logout()}>Log out</button>
      </div>
    </aside>

    <main class="workspace-main">
      <header class="workspace-header">
          <button class="mobile-menu-trigger" type="button" aria-label="Open navigation menu" aria-expanded={mobileMenuOpen} onclick={() => mobileMenuOpen = true}>
            <Menu size={20} strokeWidth={2} aria-hidden="true" />
          </button>
          <h1 class="workspace-breadcrumb">
            <a class="workspace-breadcrumb-segment" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(workspacePath(activeWorkspace)) } }}><Building size={16} strokeWidth={2} aria-hidden="true" /><span>{activeWorkspace?.name ?? "New Workspace"}</span></a>
            {#if isChatCollection()}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              <span>Chats</span>
            {:else if isProjectCollection()}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              <span>Projects</span>
            {:else if isGroupCollection()}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              <span>Groups</span>
            {:else if activeProject !== null}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
				{#if activeSession === null}
                <span class="workspace-breadcrumb-segment"><Folder size={16} strokeWidth={2} aria-hidden="true" />{activeProject.name ?? "New Project"}</span>
              {:else}
                <a class="workspace-breadcrumb-segment" href={activeWorkspace !== null ? projectPath(activeWorkspace, activeProject) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(projectPath(activeWorkspace, activeProject)) } }}><Folder size={16} strokeWidth={2} aria-hidden="true" /><span>{activeProject.name ?? "New Project"}</span></a>
              {/if}
            {/if}
            {#if activeSession !== null}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              <span class="workspace-breadcrumb-segment"><MessageSquare size={16} strokeWidth={2} aria-hidden="true" />{activeSession.name ?? "New Chat"}</span>
            {/if}
          </h1>
          {#if activeSession !== null}
            <nav class="session-tabs" aria-label="Session navigation">
              <a href={activeWorkspace !== null ? sessionPath(activeWorkspace, activeSession) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionPath(activeWorkspace, activeSession)) } }}>Chat</a>
              <a href={activeWorkspace !== null ? sessionNotesPath(activeWorkspace, activeSession) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionNotesPath(activeWorkspace, activeSession)) } }}>Notes</a>
              <a href={activeWorkspace !== null ? sessionSecretsPath(activeWorkspace, activeSession) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionSecretsPath(activeWorkspace, activeSession)) } }}>Secrets</a>
            </nav>
            <select class="session-tabs-select" aria-label="Session view" value="chat" onchange={(event) => { if (activeWorkspace !== null) { navigate(event.currentTarget.value === "notes" ? sessionNotesPath(activeWorkspace, activeSession) : event.currentTarget.value === "secrets" ? sessionSecretsPath(activeWorkspace, activeSession) : sessionPath(activeWorkspace, activeSession)) } }}>
              <option value="chat">Chat</option>
              <option value="notes">Notes</option>
              <option value="secrets">Secrets</option>
            </select>
          {/if}
      </header>
      {#if routeSessionID(route) !== null && activeSession?.id !== routeSessionID(route)}
        <p class="dashboard-empty" aria-busy="true" aria-live="polite">Loading chat...</p>
      {:else if activeSession === null && activeProject === null && !isChatCollection() && !isProjectCollection() && !isGroupCollection()}
        {#if activeWorkspace !== null}<WorkspaceDashboardPage workspace={activeWorkspace} sessions={latestSessions} projects={latestProjects} creatingProject={creatingProject} error={messageError} onCreateSession={() => void createSession()} onCreateProject={() => void createProject()} onNavigate={navigate} />{/if}
      {:else if isChatCollection()}
        {#if activeWorkspace !== null}<ChatCollectionPage workspace={activeWorkspace} search={route.kind === "session-collection" ? route.search : ""} onAuthenticationLost={signInRequired} onCreate={() => void createSession()} onNavigate={navigate} />{/if}
      {:else if isProjectCollection()}
        {#if activeWorkspace !== null}<ProjectCollectionPage workspace={activeWorkspace} search={route.kind === "project-collection" ? route.search : ""} creating={creatingProject} onAuthenticationLost={signInRequired} onCreate={() => void createProject()} onNavigate={navigate} />{/if}
      {:else if isGroupCollection()}
        {#if activeWorkspace !== null}<GroupsPage workspace={activeWorkspace} {activity} onAuthenticationLost={signInRequired} />{/if}
      {:else if activeSession === null}
        <p class="dashboard-empty">This page is unavailable.</p>
      {:else}
        <p class="dashboard-empty">This page is unavailable.</p>
      {/if}
    </main>
  </div>
{/if}
