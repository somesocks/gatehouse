<script lang="ts">
  import { onMount } from "svelte"
  import { Building, Folder, Lock, Menu, MessageSquare, NotebookPen, Paperclip, X } from "@lucide/svelte"
  import type { ActivitySelector } from "./utils/activity-poller"
  import type { Workspace } from "./app/access"
  import { createActivityClient } from "./app/activity"
  import { createAccess } from "./app/access.svelte"
  import { signOut } from "./app/auth"
  import { createAuth } from "./app/auth.svelte"
  import { createRouter } from "./app/router"
  import { type NoteAuthor, type ProjectNote } from "./app/project-notes"
  import { fetchProjectSecrets, type ProjectSecret } from "./app/project-secrets"
  import ChatCollectionPage from "./pages/chats/ChatCollectionPage.svelte"
  import ChatPage from "./pages/chat/ChatPage.svelte"
  import GroupsPage from "./pages/groups/GroupsPage.svelte"
  import LoginPage from "./pages/login/LoginPage.svelte"
  import ProjectCollectionPage from "./pages/projects/ProjectCollectionPage.svelte"
  import ProjectNotesPage from "./pages/project-notes/ProjectNotesPage.svelte"
  import ProjectSecretsPage from "./pages/project-secrets/ProjectSecretsPage.svelte"
  import SessionSecretsPage from "./pages/session-secrets/SessionSecretsPage.svelte"
  import SessionNotesPage from "./pages/session-notes/SessionNotesPage.svelte"
  import WorkspaceDashboardPage from "./pages/workspace/WorkspaceDashboardPage.svelte"
  import SystemPage from "./pages/system/SystemPage.svelte"
  import { routeProjectID, routeSessionID, routeWorkspaceID } from "./route"
  import type { Route } from "./route"

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

  type ProjectFile = {
    id: string
    name: string
    media_type?: string
    size: number
    fingerprint: string
    created_at: string
  }

  type NoteRevisionSummary = {
    revision: number
    title: string
    description: string
    sensitive: boolean
    author: NoteAuthor
    created_at: string
  }

  type NoteRevision = NoteRevisionSummary & {
    body?: string
  }

  function noteAuthorLabel(author: NoteAuthor): string {
    if (author.principal !== undefined) {
      return author.principal.name ?? author.principal.id
    }
    if (author.agent !== undefined) {
      return author.agent.label ?? author.agent.id
    }
    return author.gateway ?? "Unknown"
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

  const auth = createAuth()
  let workspaceContentStatus = $state<WorkspaceContentStatus>("checking")
  let route = $state<Route>({ kind: "app-home" })
  let activeWorkspace = $state<Workspace | null>(null)
  let latestProjects = $state<Project[]>([])
  let latestSessions = $state<Session[]>([])
  let activeSession = $state<Session | null>(null)
  let activeProject = $state<Project | null>(null)
  let noteHistoryNoteID = $state<string | null>(null)
  let noteHistoryNotesPath = $state<string | null>(null)
  let noteRevisionSummaries = $state<NoteRevisionSummary[]>([])
  let selectedNoteRevision = $state<NoteRevision | null>(null)
  let selectedNoteRevisionNumber = $state<number | null>(null)
  let noteHistoryLoading = $state(false)
  let noteRevisionLoading = $state(false)
  let noteHistoryError = $state("")
  let noteHistoryGeneration = 0
  let noteRevisionGeneration = 0
  let noteHistoryDialogElement = $state<HTMLDialogElement | undefined>()
  let projectSecretBreadcrumb = $state<string | null>(null)
  let projectNoteBreadcrumb = $state<string | null>(null)
  let projectNoteHistoryRevision = $state<number | null>(null)
  let sessionNoteBreadcrumb = $state<string | null>(null)
  let sessionSecretBreadcrumb = $state<string | null>(null)
  let creatingProject = $state(false)
  let updatingProject = $state(false)
  let projectEditName = $state("")
  let projectEditDescription = $state("")
  let projectEditError = $state("")
  let projectEditDialogElement = $state<HTMLDialogElement | undefined>()
  let projectActionMenuElement = $state<HTMLDetailsElement | undefined>()
  let messageError = $state("")
  let mobileMenuOpen = $state(false)
  let workspaceMainElement = $state<HTMLElement | undefined>()
  const activity = createActivityClient({ onAuthenticationLost: signInRequired })
  const access = createAccess({ activity, onAuthenticationLost: signInRequired })
  $effect(() => {
    const principalID = auth.state.claims?.principal.ref.id
    if (auth.state.status !== "authenticated" || principalID === undefined) {
      access.clear()
      return
    }
    return access.start(principalID)
  })
  let activeProjection: ActivityProjection | null = null
  let projectSessions = $state<Session[]>([])
  let projectFiles = $state<ProjectFile[]>([])
  let projectFileStatus = $state<WorkspaceContentStatus>("checking")
  let projectFileError = $state("")
  let uploadingProjectFiles = $state(0)
  let removingProjectFileIDs = $state<Set<string>>(new Set())
  let projectFileInputElement = $state<HTMLInputElement | undefined>()
  let projectNotes = $state<ProjectNote[]>([])
  let projectNoteStatus = $state<WorkspaceContentStatus>("checking")
	let projectSecrets = $state<ProjectSecret[]>([])
	let projectSecretStatus = $state<WorkspaceContentStatus>("checking")
  let routeGeneration = 0
  let routeAbortController: AbortController | null = null
  let initializingAuthenticatedSession = false
  const router = createRouter((next) => {
    routeAbortController?.abort()
    routeAbortController = new AbortController()
    route = next
    void activateRoute(++routeGeneration, routeAbortController.signal)
  })
  onMount(() => {
    const closeProjectActionMenu = (event: MouseEvent) => {
      if (projectActionMenuElement?.open && event.target instanceof Node && !projectActionMenuElement.contains(event.target)) {
        projectActionMenuElement.open = false
      }
    }
    const stopRouter = router.start()
    document.addEventListener("click", closeProjectActionMenu)
    resolveRoute()
    return () => {
      stopRouter()
      document.removeEventListener("click", closeProjectActionMenu)
      routeAbortController?.abort()
      stopActivityPolling()
      activity.dispose()
    }
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

  function projectNotesPath(workspace: Workspace, project: Project) {
    return `${projectPath(workspace, project)}/pnt`
  }

  function projectNotePath(workspace: Workspace, project: Project, note: ProjectNote | string) {
    const id = typeof note === "string" ? note : note.id
    return `${projectNotesPath(workspace, project)}/${encodeURIComponent(id)}`
  }

	function projectSecretsPath(workspace: Workspace, project: Project) {
		return `${projectPath(workspace, project)}/secrets`
	}

	function projectSecretPath(workspace: Workspace, project: Project, secretID: string) {
		return `${projectSecretsPath(workspace, project)}/${encodeURIComponent(secretID)}`
	}

  function groupsPath(workspace: Workspace) {
    return `${workspacePath(workspace)}/grp`
  }

  function isChatCollection() {
    return route.kind === "session-collection"
  }

  function isSessionNotesRoute() {
    return route.kind === "session-notes" || route.kind === "session-note-new" || route.kind === "session-note" || route.kind === "session-note-edit" || route.kind === "session-note-revision"
  }

  function isSessionSecretsRoute() {
    return route.kind === "session-secrets" || route.kind === "session-secret-new" || route.kind === "session-secret"
  }

  function isProjectNotesRoute() {
    return route.kind === "project-notes" || route.kind === "project-note-new" || route.kind === "project-note"
  }

  function isProjectSecretsRoute() {
    return route.kind === "project-secrets" || route.kind === "project-secret-new" || route.kind === "project-secret"
  }

  function isProjectCollection() {
    return route.kind === "project-collection"
  }

  function isGroupCollection() {
    return route.kind === "group-collection"
  }

  function createdAtLabel(value: string) {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) {
      return value
    }
    const number = (part: number) => part.toString().padStart(2, "0")
    return `${date.getFullYear()}-${number(date.getMonth() + 1)}-${number(date.getDate())} ${number(date.getHours())}:${number(date.getMinutes())}`
  }

  function clearNoteHistoryPicker() {
    noteHistoryGeneration += 1
    noteRevisionGeneration += 1
    noteHistoryNoteID = null
    noteHistoryNotesPath = null
    noteRevisionSummaries = []
    noteHistoryLoading = false
    noteRevisionLoading = false
    noteHistoryError = ""
    sessionNoteHistorySelection = null
  }

  function resetNoteHistory() {
    clearNoteHistoryPicker()
    selectedNoteRevision = null
    selectedNoteRevisionNumber = null
    projectNoteHistoryRevision = null
  }

  function openNoteHistory(notesPath: string, noteID: string) {
    if (!noteHistoryDialogElement?.open) {
      noteHistoryDialogElement?.showModal()
    }
    noteHistoryNotesPath = notesPath
    void loadNoteHistory(notesPath, noteID)
  }

  function openProjectNoteHistory(notesPath: string, noteID: string, revision: number) {
    projectNoteHistoryRevision = revision
    openNoteHistory(notesPath, noteID)
  }

  let sessionNoteHistorySelection = $state<((revision: number) => void) | null>(null)

  function openSessionNoteHistory(notesPath: string, noteID: string, onRevision: (revision: number) => void) {
    projectNoteHistoryRevision = null
    openNoteHistory(notesPath, noteID)
    sessionNoteHistorySelection = onRevision
  }

  function closeNoteHistory() {
    clearNoteHistoryPicker()
  }

  function showCurrentNoteRevision() {
    selectedNoteRevision = null
    selectedNoteRevisionNumber = null
    noteHistoryDialogElement?.close()
  }

  async function loadNoteHistory(notesPath: string, noteID: string) {
    clearNoteHistoryPicker()
    const historyGeneration = noteHistoryGeneration
    noteHistoryNoteID = noteID
    noteHistoryLoading = true
    try {
      const response = await fetch(`${notesPath}/${encodeURIComponent(noteID)}/revisions`, { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return false
      }
      if (!response.ok) {
        throw new Error("note history could not be loaded")
      }
      const revisions = (await response.json()) as NoteRevisionSummary[]
      if (historyGeneration !== noteHistoryGeneration || noteHistoryNoteID !== noteID) {
        return
      }
      noteRevisionSummaries = [...revisions].sort((left, right) => right.revision - left.revision)
    } catch {
      if (historyGeneration === noteHistoryGeneration && noteHistoryNoteID === noteID) {
        noteHistoryError = "The note history could not be loaded. Try again."
      }
    } finally {
      if (historyGeneration === noteHistoryGeneration && noteHistoryNoteID === noteID) {
        noteHistoryLoading = false
      }
    }
  }

  async function loadNoteRevision(notesPath: string, noteID: string, revision: number) {
    if (noteHistoryNoteID !== noteID) {
      return
    }
    const revisionGeneration = ++noteRevisionGeneration
    selectedNoteRevisionNumber = revision
    selectedNoteRevision = null
    noteRevisionLoading = true
    noteHistoryError = ""
    try {
      const response = await fetch(`${notesPath}/${encodeURIComponent(noteID)}/revisions/${encodeURIComponent(revision)}`, { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("note revision could not be loaded")
      }
      const loaded = (await response.json()) as NoteRevision
      if (revisionGeneration !== noteRevisionGeneration || noteHistoryNoteID !== noteID) {
        return
      }
      selectedNoteRevision = loaded
      noteHistoryDialogElement?.close()
    } catch {
      if (revisionGeneration === noteRevisionGeneration && noteHistoryNoteID === noteID) {
        noteHistoryError = "The note revision could not be loaded. Try again."
      }
    } finally {
      if (revisionGeneration === noteRevisionGeneration && noteHistoryNoteID === noteID) {
        noteRevisionLoading = false
      }
    }
  }

  function navigate(path: string, replace = false) {
    router.navigate(path, replace)
  }

  function resolveRoute() {
    router.resolve()
  }

  function isCurrentRoute(generation: number) {
    return generation === routeGeneration
  }

  function redirectToLogin() {
    const requested = window.location.pathname + window.location.search + window.location.hash
    navigate(`/app/login?next=${encodeURIComponent(requested)}`, true)
  }

  function signInRequired() {
    routeAbortController?.abort()
    routeGeneration += 1
    stopActivityPolling()
    activity.dispose()
    resetNoteHistory()
    auth.clear()
    access.clear()
    activeWorkspace = null
    latestProjects = []
    latestSessions = []
    activeSession = null
    sessionNoteBreadcrumb = null
    sessionSecretBreadcrumb = null
    activeProject = null
    projectNoteBreadcrumb = null
    projectSecretBreadcrumb = null
    projectFiles = []
    projectFileError = ""
    projectNotes = []
		projectSecrets = []
    if (!isLoginPath()) {
      redirectToLogin()
    }
  }

  async function checkSession() {
    if (initializingAuthenticatedSession) {
      return
    }
    initializingAuthenticatedSession = true
    try {
      const next = await auth.check()
      if (next === "anonymous") {
        signInRequired()
        return
      }
      if (next === "authenticated") {
        await loadWorkspaces()
        configureActivityPolling()
        await activity.poll()
      }
    } finally {
      initializingAuthenticatedSession = false
    }
  }

  async function loadWorkspaces(resolveAfterLoad = true) {
    if (!await access.refresh()) {
      return false
    }
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
    if (resolveAfterLoad) {
      resolveRoute()
    }
    return true
  }

  async function activateRoute(generation: number, signal: AbortSignal) {
    if (auth.state.status !== "authenticated") {
      void checkSession()
      return
    }
    if (isSystemRoute()) {
      return
    }
    if (access.state.workspaceStatus !== "ready") {
      void loadWorkspaces()
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
    await activity.poll()
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

  async function loadInitialProjectProjection(workspace: Workspace, project: Project) {
    await Promise.all([loadProjectSessions(project), loadProjectFiles(project), loadProjectNotes(project), loadProjectSecrets(project)])
  }

  async function activateWorkspace(workspace: Workspace, generation: number, signal: AbortSignal) {
    if (activeWorkspace?.id === workspace.id) {
      activeWorkspace = workspace
      return true
    }
    mobileMenuOpen = false
    stopActivityPolling()
    activeWorkspace = workspace
    resetNoteHistory()
    latestProjects = []
    latestSessions = []
    activeSession = null
    sessionNoteBreadcrumb = null
    sessionSecretBreadcrumb = null
    activeProject = null
    projectNoteBreadcrumb = null
    projectSecretBreadcrumb = null
    projectFiles = []
    projectFileError = ""
    projectNotes = []
		projectSecrets = []
    workspaceContentStatus = "checking"
    return !signal.aborted && isCurrentRoute(generation)
  }

  function activateWorkspacePage() {
    stopActivityPolling()
    activeSession = null
    activeProject = null
    projectNoteBreadcrumb = null
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
    resetNoteHistory()
    sessionNoteBreadcrumb = null
    sessionSecretBreadcrumb = null
    projectNoteBreadcrumb = null
    projectFiles = []
    projectNotes = []
		projectSecrets = []
  }

  async function activateProject(project: Project, generation: number) {
    if (activeWorkspace === null) {
      return
    }
    mobileMenuOpen = false
    stopActivityPolling()
    activeSession = null
    activeProject = project
    resetNoteHistory()
    projectNoteBreadcrumb = null
    projectSecretBreadcrumb = null
    projectFiles = []
    projectFileError = ""
    projectNotes = []
		projectSecrets = []
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

  async function loadProjectSessions(project: Project, projection?: ActivityProjection) {
    if (activeWorkspace === null) {
      return
    }
    const workspace = activeWorkspace
    const parameters = new URLSearchParams({ limit: "5", project: project.id })
    const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions?${parameters}`, { credentials: "same-origin" })
    if (response.status === 401) {
      signInRequired()
      return
    }
    if (!response.ok) {
      throw new Error("project sessions could not be loaded")
    }
    const loaded = (await response.json()) as SessionSearchResponse
    if ((projection === undefined || isActiveProjection(projection)) && activeWorkspace?.id === workspace.id && activeProject?.id === project.id && activeSession === null) {
      projectSessions = loaded.sessions
    }
  }

  async function loadProjectFiles(project: Project, showLoading = true, projection?: ActivityProjection) {
    if (activeWorkspace === null) {
      return false
    }
    const workspace = activeWorkspace
    if (showLoading) {
      projectFileStatus = "checking"
    }
    try {
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects/${encodeURIComponent(project.id)}/files`, { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return false
      }
      if (!response.ok) {
        throw new Error("project files could not be loaded")
      }
      const loaded = (await response.json()) as ProjectFile[]
      if (projection !== undefined && !isActiveProjection(projection) || activeWorkspace?.id !== workspace.id || activeProject?.id !== project.id || activeSession !== null) {
        return false
      }
      projectFiles = [...loaded].sort((left, right) => {
        const difference = new Date(right.created_at).getTime() - new Date(left.created_at).getTime()
        return Number.isFinite(difference) && difference !== 0 ? difference : right.id.localeCompare(left.id)
      })
      projectFileStatus = "ready"
      return true
    } catch {
      if ((projection === undefined || isActiveProjection(projection)) && activeWorkspace?.id === workspace.id && activeProject?.id === project.id && activeSession === null) {
        projectFileStatus = "unavailable"
      }
      return false
    }
  }

  async function loadProjectNotes(project: Project, showLoading = true, projection?: ActivityProjection) {
    if (activeWorkspace === null) {
      return false
    }
    const workspace = activeWorkspace
    if (showLoading) {
      projectNoteStatus = "checking"
    }
    try {
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects/${encodeURIComponent(project.id)}/notes`, { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return false
      }
      if (!response.ok) {
        throw new Error("project notes could not be loaded")
      }
      const loaded = (await response.json()) as ProjectNote[]
      if (projection !== undefined && !isActiveProjection(projection) || activeWorkspace?.id !== workspace.id || activeProject?.id !== project.id || activeSession !== null) {
        return false
      }
      projectNotes = [...loaded].sort((left, right) => {
        const difference = new Date(right.created_at).getTime() - new Date(left.created_at).getTime()
        return Number.isFinite(difference) && difference !== 0 ? difference : right.id.localeCompare(left.id)
      })
      projectNoteStatus = "ready"
      return true
    } catch {
      if ((projection === undefined || isActiveProjection(projection)) && activeWorkspace?.id === workspace.id && activeProject?.id === project.id && activeSession === null) {
        projectNoteStatus = "unavailable"
      }
      return false
    }
  }

	async function loadProjectSecrets(project: Project, showLoading = true, projection?: ActivityProjection) {
		if (activeWorkspace === null) {
			return false
		}
		const workspace = activeWorkspace
		if (showLoading) {
			projectSecretStatus = "checking"
		}
		try {
			const response = await fetchProjectSecrets(workspace.id, project.id)
			if (response.status === 401) {
				signInRequired()
				return false
			}
			if (!response.ok) {
				throw new Error("project secrets could not be loaded")
			}
			const loaded = (await response.json()) as ProjectSecret[]
			if (projection !== undefined && !isActiveProjection(projection) || activeWorkspace?.id !== workspace.id || activeProject?.id !== project.id || activeSession !== null) {
				return false
			}
			projectSecrets = [...loaded].sort((left, right) => {
				const difference = new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime()
				return Number.isFinite(difference) && difference !== 0 ? difference : right.id.localeCompare(left.id)
			})
			projectSecretStatus = "ready"
			return true
		} catch {
			if ((projection === undefined || isActiveProjection(projection)) && activeWorkspace?.id === workspace.id && activeProject?.id === project.id && activeSession === null) {
				projectSecretStatus = "unavailable"
			}
			return false
		}
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
    const agentSelector = session === null || route.kind === "session-chat" ? undefined : {
      name: "agent",
      topic: workspace.id,
      events: ["workspace_agent.*"],
    } satisfies ActivitySelector
    const sessionSelector = session === null ? (overview ? {
      name: "session",
      topic: workspace.id,
      events: ["session.*"],
    } satisfies ActivitySelector : undefined) : route.kind === "session-chat" ? undefined : {
      name: "session",
      topic: `${workspace.id}/${session.id}`,
      events: ["session.*", "session_event.*", "session_file.*", ...(isSessionNotesRoute() ? [] : ["session_note.*"]), ...(isSessionSecretsRoute() ? [] : ["session_secret.*"])],
    } satisfies ActivitySelector
    const projectID = session?.project?.id ?? activeProject?.id
    const projectSelector = projectID === undefined ? (overview ? {
      name: "project",
      topic: workspace.id,
      events: ["project.*"],
    } satisfies ActivitySelector : undefined) : {
      name: "project",
      topic: `${workspace.id}/${projectID}`,
      events: ["project.*", "project_file.*", ...(isProjectNotesRoute() ? [] : ["project_note.*"]), ...(isProjectSecretsRoute() ? [] : ["project_secret.*"]), "session.*"],
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
      const projectChanged = projectSelector !== undefined && names.has(projectSelector.name)

      let refreshed = (await Promise.all([
        ...(workspaceChanged ? [loadWorkspaces(false)] : []),
        ...(sessionSelector !== undefined ? [refreshWorkspaceSessions(workspace, projection)] : []),
        ...(projectSelector !== undefined ? [refreshWorkspaceProjects(workspace, projection)] : []),
      ])).every(Boolean)
      if (!isActiveProjection(projection) || signal.aborted) {
        throw new Error("activity projection refresh failed")
      }
      if (refreshed && session === null && projectChanged && activeProject !== null && activeProject.id === projectID) {
		refreshed = (await Promise.all([loadProjectSessions(activeProject, projection), loadProjectFiles(activeProject, false, projection), ...(isProjectNotesRoute() ? [] : [loadProjectNotes(activeProject, false, projection)]), loadProjectSecrets(activeProject, false, projection)])).every(Boolean)
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

  function startProjectNoteCreate() {
    if (activeWorkspace !== null && activeProject !== null) {
      navigate(projectNotePath(activeWorkspace, activeProject, "new"))
    }
  }

  function openProjectEdit() {
    if (activeProject === null) {
      return
    }
    projectActionMenuElement?.removeAttribute("open")
    projectEditName = activeProject.name ?? ""
    projectEditDescription = activeProject.description ?? ""
    projectEditError = ""
    projectEditDialogElement?.showModal()
  }

  function closeProjectEdit() {
    if (!updatingProject) {
      projectEditDialogElement?.close()
    }
  }

  async function updateProject() {
    if (activeWorkspace === null || activeProject === null) {
      return
    }
    const workspace = activeWorkspace
    const project = activeProject
    projectEditError = ""
    updatingProject = true
    try {
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects/${encodeURIComponent(project.id)}`, {
        method: "PATCH",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: projectEditName, description: projectEditDescription }),
      })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("project could not be updated")
      }
      const updated = (await response.json()) as Project
      if (activeWorkspace?.id !== workspace.id || activeProject?.id !== project.id) {
        return
      }
      activeProject = updated
      latestProjects = latestProjects.map((candidate) => candidate.id === updated.id ? updated : candidate)
      projectEditDialogElement?.close()
    } catch {
      projectEditError = "The project could not be updated. Try again."
    } finally {
      updatingProject = false
    }
  }

  async function uploadProjectFiles(input: HTMLInputElement) {
    if (activeWorkspace === null || activeProject === null) {
      return
    }
    const selected = Array.from(input.files ?? [])
    input.value = ""
    if (selected.length === 0) {
      return
    }
    const workspace = activeWorkspace
    const project = activeProject
    projectFileError = ""
    uploadingProjectFiles += selected.length
    try {
      const results = await Promise.allSettled(selected.map((file) => uploadProjectFile(workspace, project, file)))
      await loadProjectFiles(project, false)
      if (results.some((result) => result.status === "rejected")) {
        projectFileError = "Some files could not be uploaded. Try again."
      }
    } finally {
      uploadingProjectFiles -= selected.length
    }
  }

  async function uploadProjectFile(workspace: Workspace, project: Project, file: File) {
    const created = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects/${encodeURIComponent(project.id)}/files/start`, {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name: file.name, ...(file.type === "" ? {} : { media_type: file.type }) }),
    })
    if (created.status === 401) {
      signInRequired()
      throw new Error("authentication required")
    }
    if (!created.ok) {
      throw new Error("project file could not be started")
    }
    const upload = (await created.json()) as { file: ProjectFile; upload_url: string }
    const put = await fetch(upload.upload_url, { method: "PUT", body: file, ...(file.type === "" ? {} : { headers: { "Content-Type": file.type } }) })
    if (!put.ok) {
      throw new Error("project file could not be uploaded")
    }
    const finished = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects/${encodeURIComponent(project.id)}/files/${encodeURIComponent(upload.file.id)}/finish`, {
      method: "POST",
      credentials: "same-origin",
    })
    if (finished.status === 401) {
      signInRequired()
      throw new Error("authentication required")
    }
    if (!finished.ok) {
      throw new Error("project file could not be finished")
    }
  }

  async function removeProjectFile(file: ProjectFile) {
    if (activeWorkspace === null || activeProject === null || removingProjectFileIDs.has(file.id) || !window.confirm(`Remove ${file.name}?`)) {
      return
    }
    const workspace = activeWorkspace
    const project = activeProject
    const removing = new Set(removingProjectFileIDs)
    removing.add(file.id)
    removingProjectFileIDs = removing
    projectFileError = ""
    try {
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects/${encodeURIComponent(project.id)}/files/${encodeURIComponent(file.id)}`, {
        method: "DELETE",
        credentials: "same-origin",
      })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("project file could not be removed")
      }
      if (activeWorkspace?.id === workspace.id && activeProject?.id === project.id && activeSession === null) {
        projectFiles = projectFiles.filter((candidate) => candidate.id !== file.id)
      }
    } catch {
      projectFileError = "The file could not be removed. Try again."
    } finally {
      const remaining = new Set(removingProjectFileIDs)
      remaining.delete(file.id)
      removingProjectFileIDs = remaining
    }
  }

  function projectFileDownloadPath(workspace: Workspace, project: Project, file: ProjectFile) {
    return `/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects/${encodeURIComponent(project.id)}/files/${encodeURIComponent(file.id)}/download`
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
      <button class="button is-primary" type="button" onclick={() => void checkSession()}>Try again</button>
    </section>
  </main>
{:else if auth.state.status === "anonymous"}
  <LoginPage {auth} onAuthenticated={resolveRoute} />
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

    <main class="workspace-main" bind:this={workspaceMainElement}>
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
				{#if activeSession === null && !isProjectNotesRoute() && !isProjectSecretsRoute()}
                <span class="workspace-breadcrumb-segment"><Folder size={16} strokeWidth={2} aria-hidden="true" />{activeProject.name ?? "New Project"}</span>
              {:else}
                <a class="workspace-breadcrumb-segment" href={activeWorkspace !== null ? projectPath(activeWorkspace, activeProject) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(projectPath(activeWorkspace, activeProject)) } }}><Folder size={16} strokeWidth={2} aria-hidden="true" /><span>{activeProject.name ?? "New Project"}</span></a>
              {/if}
            {/if}
            {#if activeSession !== null}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              {#if isSessionNotesRoute()}
                <a class="workspace-breadcrumb-segment" href={activeWorkspace !== null ? sessionPath(activeWorkspace, activeSession) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionPath(activeWorkspace, activeSession)) } }}><MessageSquare size={16} strokeWidth={2} aria-hidden="true" /><span>{activeSession.name ?? "New Chat"}</span></a>
                <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
                {#if route.kind !== "session-notes"}
                  <a href={activeWorkspace !== null ? sessionNotesPath(activeWorkspace, activeSession) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionNotesPath(activeWorkspace, activeSession)) } }}>Notes</a>
                {:else}
                  <span>Notes</span>
                {/if}
              {:else if isSessionSecretsRoute()}
                <a class="workspace-breadcrumb-segment" href={activeWorkspace !== null ? sessionPath(activeWorkspace, activeSession) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionPath(activeWorkspace, activeSession)) } }}><MessageSquare size={16} strokeWidth={2} aria-hidden="true" /><span>{activeSession.name ?? "New Chat"}</span></a>
                <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
                {#if route.kind === "session-secret" || route.kind === "session-secret-new"}
                  <a href={activeWorkspace !== null ? sessionSecretsPath(activeWorkspace, activeSession) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionSecretsPath(activeWorkspace, activeSession)) } }}>Secrets</a>
                {:else}
                  <span>Secrets</span>
                {/if}
              {:else}
                <span class="workspace-breadcrumb-segment"><MessageSquare size={16} strokeWidth={2} aria-hidden="true" />{activeSession.name ?? "New Chat"}</span>
              {/if}
			{:else if activeProject !== null && isProjectSecretsRoute()}
				<span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
				{#if route.kind === "project-secret" || route.kind === "project-secret-new"}
                  <a href={activeWorkspace !== null ? projectSecretsPath(activeWorkspace, activeProject) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(projectSecretsPath(activeWorkspace, activeProject)) } }}>Secrets</a>
                {:else}
                  <span>Secrets</span>
				{/if}
			{:else if activeProject !== null && isProjectNotesRoute()}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              {#if route.kind === "project-note" || route.kind === "project-note-new"}
                <a href={activeWorkspace !== null ? projectNotesPath(activeWorkspace, activeProject) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(projectNotesPath(activeWorkspace, activeProject)) } }}>Notes</a>
              {:else}
                <span>Notes</span>
              {/if}
              {#if route.kind === "project-note" || route.kind === "project-note-new"}
                <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
                <span class="workspace-breadcrumb-segment"><NotebookPen size={16} strokeWidth={2} aria-hidden="true" />{projectNoteBreadcrumb ?? (route.kind === "project-note-new" ? "New Note" : "Note")}</span>
              {/if}
            {/if}
            {#if activeSession !== null && sessionNoteBreadcrumb !== null}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              <span class="workspace-breadcrumb-segment"><NotebookPen size={16} strokeWidth={2} aria-hidden="true" />{sessionNoteBreadcrumb}</span>
            {/if}
            {#if activeSession !== null && sessionSecretBreadcrumb !== null}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              <span class="workspace-breadcrumb-segment"><Lock size={16} strokeWidth={2} aria-hidden="true" />{sessionSecretBreadcrumb}</span>
            {/if}
			{#if activeSession === null && activeProject !== null && projectSecretBreadcrumb !== null}
				<span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
                <span class="workspace-breadcrumb-segment"><Lock size={16} strokeWidth={2} aria-hidden="true" />{projectSecretBreadcrumb}</span>
			{/if}
          </h1>
          {#if activeSession !== null}
            <nav class="session-tabs" aria-label="Session navigation">
              <a class:active={!isSessionNotesRoute() && !isSessionSecretsRoute()} href={activeWorkspace !== null ? sessionPath(activeWorkspace, activeSession) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionPath(activeWorkspace, activeSession)) } }}>Chat</a>
              <a class:active={isSessionNotesRoute()} href={activeWorkspace !== null ? sessionNotesPath(activeWorkspace, activeSession) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionNotesPath(activeWorkspace, activeSession)) } }}>Notes</a>
              <a class:active={isSessionSecretsRoute()} href={activeWorkspace !== null ? sessionSecretsPath(activeWorkspace, activeSession) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionSecretsPath(activeWorkspace, activeSession)) } }}>Secrets</a>
            </nav>
            <select class="session-tabs-select" aria-label="Session view" value={isSessionNotesRoute() ? "notes" : isSessionSecretsRoute() ? "secrets" : "chat"} onchange={(event) => { if (activeWorkspace !== null) { navigate(event.currentTarget.value === "notes" ? sessionNotesPath(activeWorkspace, activeSession) : event.currentTarget.value === "secrets" ? sessionSecretsPath(activeWorkspace, activeSession) : sessionPath(activeWorkspace, activeSession)) } }}>
              <option value="chat">Chat</option>
              <option value="notes">Notes</option>
              <option value="secrets">Secrets</option>
            </select>
          {/if}
      </header>
		{#if activeSession === null && activeProject !== null && !isProjectNotesRoute() && !isProjectSecretsRoute()}
        <section class="project-dashboard-heading">
          <div>
            <h2 class="title is-3">{activeProject.name ?? "New Project"}</h2>
            <p class="subtitle is-6">{activeProject.description ?? "No description yet."}</p>
          </div>
          <details class="project-action-menu" bind:this={projectActionMenuElement}>
            <summary class="button is-small project-action-menu-trigger" aria-label="Project actions" title="Project actions"><Menu size={22} strokeWidth={2} aria-hidden="true" /></summary>
				<div class="project-action-menu-items"><button type="button" onclick={openProjectEdit}>Edit project</button></div>
          </details>
        </section>
      {/if}
      {#if activeSession === null && activeProject === null && !isChatCollection() && !isProjectCollection() && !isGroupCollection()}
        {#if activeWorkspace !== null}<WorkspaceDashboardPage workspace={activeWorkspace} sessions={latestSessions} projects={latestProjects} creatingProject={creatingProject} error={messageError} onCreateSession={() => void createSession()} onCreateProject={() => void createProject()} onNavigate={navigate} />{/if}
      {:else if isChatCollection()}
        {#if activeWorkspace !== null}<ChatCollectionPage workspace={activeWorkspace} search={route.kind === "session-collection" ? route.search : ""} onAuthenticationLost={signInRequired} onCreate={() => void createSession()} onNavigate={navigate} />{/if}
      {:else if isProjectCollection()}
        {#if activeWorkspace !== null}<ProjectCollectionPage workspace={activeWorkspace} search={route.kind === "project-collection" ? route.search : ""} creating={creatingProject} onAuthenticationLost={signInRequired} onCreate={() => void createProject()} onNavigate={navigate} />{/if}
      {:else if isGroupCollection()}
        {#if activeWorkspace !== null}<GroupsPage workspace={activeWorkspace} {activity} onAuthenticationLost={signInRequired} />{/if}
      {:else if activeSession === null && activeProject !== null && isProjectNotesRoute()}
			<ProjectNotesPage workspaceID={activeWorkspace!.id} projectID={activeProject.id} {route} {activity} previewNotes={projectNotes} previewStatus={projectNoteStatus} selectedRevision={selectedNoteRevision} onAuthenticationLost={signInRequired} onNavigate={navigate} onBreadcrumbChange={(title) => projectNoteBreadcrumb = title} onPreviewChanged={() => loadProjectNotes(activeProject!, false)} onResetHistory={resetNoteHistory} onOpenHistory={openProjectNoteHistory} onShowCurrentRevision={showCurrentNoteRevision} />
      {:else if activeSession === null && activeProject !== null && isProjectSecretsRoute()}
			<ProjectSecretsPage workspaceID={activeWorkspace!.id} projectID={activeProject.id} {route} {activity} onAuthenticationLost={signInRequired} onNavigate={navigate} onBreadcrumbChange={(title) => projectSecretBreadcrumb = title} onChanged={() => void loadProjectSecrets(activeProject!, false)} />
      {:else if activeSession !== null && isSessionNotesRoute()}
        <SessionNotesPage workspaceID={activeWorkspace!.id} sessionID={activeSession.id} {route} {activity} onAuthenticationLost={signInRequired} onNavigate={navigate} onBreadcrumbChange={(title) => sessionNoteBreadcrumb = title} onResetHistory={resetNoteHistory} onOpenHistory={openSessionNoteHistory} />
      {:else if activeSession !== null && isSessionSecretsRoute()}
        <SessionSecretsPage workspaceID={activeWorkspace!.id} sessionID={activeSession.id} {route} {activity} onAuthenticationLost={signInRequired} onNavigate={navigate} onBreadcrumbChange={(title) => sessionSecretBreadcrumb = title} />
      {:else if activeSession === null}
        <section class="dashboard-grid">
          <section class="dashboard-widget dashboard-widget-wide">
            <div class="dashboard-widget-heading"><h2>Project Chats</h2><button class="button is-primary is-small" type="button" onclick={() => void createSession(activeProject ?? undefined)}>New chat</button></div>
            {#each projectSessions as session}
              <a class="dashboard-row" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses/${encodeURIComponent(session.id)}`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionPath(activeWorkspace, session)) } }}><span class="dashboard-row-content"><span>{session.name ?? "New Chat"}</span><span class="dashboard-row-meta"><time datetime={session.created_at}>{createdAtLabel(session.created_at)}</time>{#if session.project !== undefined}<span aria-hidden="true">/</span><span>{session.project.name ?? "New Project"}</span>{/if}</span></span></a>
            {:else}<p class="dashboard-empty">No project chats yet.</p>{/each}
            {#if projectSessions.length > 0}<a class="dashboard-view-all" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionsPath(activeWorkspace)) } }}>View all chats</a>{/if}
          </section>
          <section class="dashboard-widget dashboard-widget-wide project-files-widget">
            <div class="dashboard-widget-heading"><h2>Project Files</h2><button class="button is-primary is-small" type="button" disabled={uploadingProjectFiles > 0} onclick={() => projectFileInputElement?.click()}>{uploadingProjectFiles > 0 ? "Uploading..." : "Upload files"}</button></div>
            <input class="is-sr-only" type="file" autocomplete="off" multiple bind:this={projectFileInputElement} onchange={(event) => void uploadProjectFiles(event.currentTarget)} />
            {#if projectFileStatus === "checking"}
              <p class="dashboard-empty">Loading files...</p>
            {:else if projectFileStatus === "unavailable"}
              <p class="dashboard-empty">Files could not be loaded.</p>
            {:else}
              {#each projectFiles as file (file.id)}
                <div class="project-file-row">
                  <a class="project-file-download" href={activeWorkspace !== null && activeProject !== null ? projectFileDownloadPath(activeWorkspace, activeProject, file) : "#"} download={file.name} title={file.fingerprint}>
                    <Paperclip size={16} strokeWidth={2} aria-hidden="true" />
                    <span class="project-file-content"><span>{file.name}</span><span class="project-file-meta"><time datetime={file.created_at}>{createdAtLabel(file.created_at)}</time><span>{file.size} bytes</span>{#if file.media_type !== undefined}<span>{file.media_type}</span>{/if}</span></span>
                  </a>
                  <button class="button is-small is-danger is-light" type="button" disabled={removingProjectFileIDs.has(file.id)} onclick={() => void removeProjectFile(file)}>{removingProjectFileIDs.has(file.id) ? "Removing..." : "Remove"}</button>
                </div>
              {:else}<p class="dashboard-empty">No files yet.</p>{/each}
            {/if}
            {#if projectFileError !== ""}<p class="help is-danger" aria-live="polite">{projectFileError}</p>{/if}
          </section>
          <section class="dashboard-widget dashboard-widget-wide project-notes-widget">
            <div class="dashboard-widget-heading"><h2>Project Notes</h2><button class="button is-primary is-small" type="button" onclick={startProjectNoteCreate}>New note</button></div>
            {#if projectNoteStatus === "checking"}
              <p class="dashboard-empty">Loading notes...</p>
            {:else if projectNoteStatus === "unavailable"}
              <p class="dashboard-empty">Notes could not be loaded.</p>
            {:else}
              {#each projectNotes as note (note.id)}
                <a class="dashboard-row project-note-row" href={activeWorkspace !== null && activeProject !== null ? projectNotePath(activeWorkspace, activeProject, note) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null && activeProject !== null) { navigate(projectNotePath(activeWorkspace, activeProject, note)) } }}><span class="dashboard-row-content"><span class="project-note-title">{note.title}{#if note.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span>{#if note.description !== ""}<span class="project-note-description">{note.description}</span>{/if}<span class="dashboard-row-meta"><time datetime={note.created_at}>{createdAtLabel(note.created_at)}</time></span></span></a>
              {:else}<p class="dashboard-empty">No notes yet.</p>{/each}
            {/if}
          </section>
			<section class="dashboard-widget dashboard-widget-wide project-notes-widget">
				<div class="dashboard-widget-heading"><h2>Project Secrets</h2><button class="button is-primary is-small" type="button" onclick={() => { if (activeWorkspace !== null && activeProject !== null) navigate(`${projectSecretsPath(activeWorkspace, activeProject)}/new`) }}>New secret</button></div>
				{#if projectSecretStatus === "checking"}
					<p class="dashboard-empty">Loading secrets...</p>
				{:else if projectSecretStatus === "unavailable"}
					<p class="dashboard-empty">Secrets could not be loaded.</p>
				{:else}
					{#each projectSecrets as secret (secret.id)}
						<a class="dashboard-row project-note-row" href={activeWorkspace !== null && activeProject !== null ? projectSecretPath(activeWorkspace, activeProject, secret.id) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null && activeProject !== null) { navigate(projectSecretPath(activeWorkspace, activeProject, secret.id)) } }}><span class="dashboard-row-content"><span class="project-note-title">{secret.description}</span><span class="dashboard-row-meta"><span>{secret.author.name ?? secret.author.id}</span><time datetime={secret.updated_at}>Updated {createdAtLabel(secret.updated_at)}</time></span></span></a>
					{:else}<p class="dashboard-empty">No secrets yet.</p>{/each}
				{/if}
				{#if projectSecrets.length > 0}<a class="dashboard-view-all" href={activeWorkspace !== null && activeProject !== null ? projectSecretsPath(activeWorkspace, activeProject) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null && activeProject !== null) { navigate(projectSecretsPath(activeWorkspace, activeProject)) } }}>View all secrets</a>{/if}
			</section>
          {#if messageError !== ""}<p class="help is-danger dashboard-error" aria-live="polite">{messageError}</p>{/if}
        </section>
      {:else if activeSession !== null && route.kind === "session-chat"}
        <ChatPage workspace={activeWorkspace!} sessionID={activeSession.id} {activity} scrollElement={workspaceMainElement} onAuthenticationLost={signInRequired} onSessionChanged={() => refreshWorkspaceSessions(activeWorkspace!)} />
      {:else}
        <p class="dashboard-empty">This page is unavailable.</p>
      {/if}
    </main>
  </div>
  <dialog class="note-history-dialog" aria-labelledby="note-history-heading" bind:this={noteHistoryDialogElement} onclose={closeNoteHistory}>
    <div class="note-history-heading"><h2 id="note-history-heading">Revision history</h2><button class="button is-ghost is-small" type="button" aria-label="Close" onclick={() => noteHistoryDialogElement?.close()}><X size={18} strokeWidth={2} aria-hidden="true" /></button></div>
    {#if noteHistoryLoading}
      <p class="dashboard-empty">Loading history...</p>
    {:else}
      {#if noteHistoryError !== ""}<p class="help is-danger" aria-live="polite">{noteHistoryError}</p>{/if}
      <div class="collection-list note-history-list">
        {#if sessionNoteHistorySelection !== null}
          {#each noteRevisionSummaries as revision (revision.revision)}
            <button class="dashboard-row project-note-row" type="button" onclick={() => { sessionNoteHistorySelection?.(revision.revision); noteHistoryDialogElement?.close() }}><span class="dashboard-row-content"><span class="project-note-title">Revision {revision.revision}: {revision.title}{#if revision.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span>{#if revision.description !== ""}<span class="project-note-description">{revision.description}</span>{/if}<span class="dashboard-row-meta"><span>{noteAuthorLabel(revision.author)}</span><time datetime={revision.created_at}>{createdAtLabel(revision.created_at)}</time></span></span></button>
          {:else}<p class="dashboard-empty">No revisions found.</p>{/each}
        {:else if activeProject !== null && projectNoteHistoryRevision !== null && noteHistoryNoteID !== null && noteHistoryNotesPath !== null}
          {#each noteRevisionSummaries as revision (revision.revision)}
            <button class="dashboard-row project-note-row" class:is-selected={selectedNoteRevisionNumber === revision.revision} type="button" disabled={noteRevisionLoading} onclick={() => revision.revision === projectNoteHistoryRevision ? showCurrentNoteRevision() : void loadNoteRevision(noteHistoryNotesPath!, noteHistoryNoteID!, revision.revision)}><span class="dashboard-row-content"><span class="project-note-title">Revision {revision.revision}: {revision.title}{#if revision.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span>{#if revision.description !== ""}<span class="project-note-description">{revision.description}</span>{/if}<span class="dashboard-row-meta"><span>{noteAuthorLabel(revision.author)}</span><time datetime={revision.created_at}>{createdAtLabel(revision.created_at)}</time></span></span></button>
          {:else}<p class="dashboard-empty">No revisions found.</p>{/each}
        {/if}
      </div>
      {#if noteRevisionLoading}<p class="dashboard-empty">Opening revision...</p>{/if}
    {/if}
  </dialog>
  <dialog class="project-edit-dialog" bind:this={projectEditDialogElement} onclose={() => projectEditError = ""}>
    <form class="project-edit-form" onsubmit={(event) => { event.preventDefault(); void updateProject() }}>
      <div class="project-edit-heading"><h2>Edit project</h2><button class="button is-ghost is-small" type="button" aria-label="Close" onclick={closeProjectEdit}><X size={18} strokeWidth={2} aria-hidden="true" /></button></div>
      <div class="field">
        <label class="label" for="project-edit-name">Name</label>
        <div class="control"><input class="input" id="project-edit-name" autocomplete="off" maxlength="256" bind:value={projectEditName} /></div>
      </div>
      <div class="field">
        <label class="label" for="project-edit-description">Description</label>
        <div class="control"><textarea class="textarea" id="project-edit-description" autocomplete="off" rows="4" maxlength="4096" bind:value={projectEditDescription}></textarea></div>
      </div>
      {#if projectEditError !== ""}<p class="help is-danger" aria-live="polite">{projectEditError}</p>{/if}
      <div class="project-edit-actions"><button class="button" type="button" disabled={updatingProject} onclick={closeProjectEdit}>Cancel</button><button class="button is-primary" type="submit" disabled={updatingProject}>{updatingProject ? "Saving..." : "Save changes"}</button></div>
    </form>
  </dialog>
{/if}
