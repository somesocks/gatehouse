<script lang="ts">
  import { onMount, tick } from "svelte"
  import { Bot, Building, CircleCheck, CircleX, Copy, Folder, Lock, Menu, MessageSquare, NotebookPen, Paperclip, Send, ShieldCheck, ShieldQuestionMark, ShieldX, X } from "@lucide/svelte"
  import type { ActivitySelector } from "./utils/activity-poller"
  import type { Workspace } from "./app/access"
  import { createActivityClient } from "./app/activity"
  import { createAccess } from "./app/access.svelte"
  import { signOut } from "./app/auth"
  import { createAuth } from "./app/auth.svelte"
  import { createRouter } from "./app/router"
  import { fetchProjectSecrets, type ProjectSecret } from "./app/project-secrets"
  import ChatCollectionPage from "./pages/chats/ChatCollectionPage.svelte"
  import GroupsPage from "./pages/groups/GroupsPage.svelte"
  import LoginPage from "./pages/login/LoginPage.svelte"
  import ProjectCollectionPage from "./pages/projects/ProjectCollectionPage.svelte"
  import ProjectSecretsPage from "./pages/project-secrets/ProjectSecretsPage.svelte"
  import SessionSecretsPage from "./pages/session-secrets/SessionSecretsPage.svelte"
  import WorkspaceDashboardPage from "./pages/workspace/WorkspaceDashboardPage.svelte"
  import SystemPage from "./pages/system/SystemPage.svelte"
  import { renderMarkdown } from "./markdown"
  import { routeProjectID, routeProjectNoteID, routeSessionID, routeSessionNoteID, routeSessionNoteRevision, routeWorkspaceID } from "./route"
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

  type NoteAuthor = {
    principal?: { id: string; name?: string }
    agent?: { id: string; label?: string }
    gateway?: string
  }

  type ProjectNote = {
    id: string
    title: string
    description: string
    body?: string
    sensitive: boolean
    author: NoteAuthor
    created_at: string
    revision: number
  }

  type SessionNote = ProjectNote & { sensitive: boolean }

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

  type WorkspaceAgent = {
    id: string
    label?: string
  }

  type SessionEvent = {
    created_at: string
    kind: string
    payload: { agent?: string; text?: string; name?: string; reason?: string; code?: string; description?: string; output?: string; attachments?: MessageFile[] }
    ref: { id: string }
    parent?: { id: string }
    author_principal?: {
      ref: { id: string }
      name?: string
    }
    author_agent?: { model: { id: string } }
  }

  type MessageFile = {
    id: string
    name: string
    media_type?: string
    size: number
    fingerprint: string
  }

  type ComposerFile = {
    file: File
    id?: string
    status: "pending" | "uploading" | "failed"
    error?: string
  }

  type SessionEventTree = {
    event: SessionEvent
    children: SessionEventTree[]
  }

  type WorkspaceContentStatus = "checking" | "ready" | "unavailable"
  type ActivityProjection = { stop: () => void }

  const auth = createAuth()
  let workspaceContentStatus = $state<WorkspaceContentStatus>("checking")
  let route = $state<Route>({ kind: "app-home" })
  let activeWorkspace = $state<Workspace | null>(null)
  let latestProjects = $state<Project[]>([])
  let agents = $state<WorkspaceAgent[]>([])
  let selectedAgent = $state("")
  let latestSessions = $state<Session[]>([])
  let activeSession = $state<Session | null>(null)
  let activeProject = $state<Project | null>(null)
  let activeProjectNote = $state<ProjectNote | null>(null)
  let creatingProjectNote = $state(false)
  let editingProjectNote = $state(false)
  let savingProjectNote = $state(false)
  let deletingProjectNote = $state(false)
  let projectNoteTitle = $state("")
  let projectNoteDescription = $state("")
  let projectNoteBody = $state("")
  let projectNoteSensitive = $state(false)
  let projectNoteError = $state("")
  let noteHistoryNoteID = $state<string | null>(null)
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
  let activeSessionNote = $state<SessionNote | null>(null)
  let creatingSessionNote = $state(false)
  let editingSessionNote = $state(false)
  let savingSessionNote = $state(false)
  let deletingSessionNote = $state(false)
  let sessionNoteTitle = $state("")
  let sessionNoteDescription = $state("")
  let sessionNoteBody = $state("")
  let sessionNoteSensitive = $state(false)
  let sessionNoteError = $state("")
  let sessionSecretBreadcrumb = $state<string | null>(null)
  let creatingProject = $state(false)
  let updatingProject = $state(false)
  let projectEditName = $state("")
  let projectEditDescription = $state("")
  let projectEditError = $state("")
  let projectEditDialogElement = $state<HTMLDialogElement | undefined>()
  let projectActionMenuElement = $state<HTMLDetailsElement | undefined>()
  let events = $state<SessionEventTree[]>([])
  let eventStatus = $state<WorkspaceContentStatus>("checking")
  let messageText = $state("")
  let composerFiles = $state<ComposerFile[]>([])
  let messageError = $state("")
  let sendingMessage = $state(false)
  let awaitingReplyFor = $state<string[]>([])
  let cancellingReplyFor = $state<Set<string>>(new Set())
  let submittingApprovals = $state<Set<string>>(new Set())
  let approvalErrors = $state<Map<string, string>>(new Map())
  let expandedActivity = $state<Set<string>>(new Set())
  let mobileMenuOpen = $state(false)
  let showJumpToLatest = $state(false)
  let workspaceMainElement = $state<HTMLElement | undefined>()
  let messageInputElement = $state<HTMLTextAreaElement | undefined>()
  let fileInputElement = $state<HTMLInputElement | undefined>()
  let activityPollTimestamp = $state(Date.now())
  const activity = createActivityClient({ onAuthenticationLost: signInRequired, onPollComplete: () => {
    activityPollTimestamp = Date.now()
  } })
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
  let sessionNotes = $state<SessionNote[]>([])
  let sessionNoteStatus = $state<WorkspaceContentStatus>("checking")
  let sessionNotePageStatus = $state<WorkspaceContentStatus | "not-found">("checking")
  let sessionNotesGeneration = 0
  let sessionNoteGeneration = 0
  let sessionNoteRevisionGeneration = 0
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
    const copyCodeBlock = (event: MouseEvent) => {
      if (!(event.target instanceof Element)) {
        return
      }
      const button = event.target.closest<HTMLButtonElement>(".markdown-code-copy")
      const code = button?.parentElement?.querySelector("pre > code")
      if (code !== null && code !== undefined) {
        void copyMarkdown(code.textContent ?? "")
      }
    }
    const stopRouter = router.start()
    document.addEventListener("click", closeProjectActionMenu)
    document.addEventListener("click", copyCodeBlock)
    resolveRoute()
    return () => {
      stopRouter()
      document.removeEventListener("click", closeProjectActionMenu)
      document.removeEventListener("click", copyCodeBlock)
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

  function sessionNotePath(workspace: Workspace, session: Session, note: SessionNote | string) {
    const id = typeof note === "string" ? note : note.id
    return `${sessionNotesPath(workspace, session)}/${encodeURIComponent(id)}`
  }

  function sessionNoteEditPath(workspace: Workspace, session: Session, note: SessionNote) {
    return `${sessionNotePath(workspace, session, note)}/edit`
  }

  function sessionNoteRevisionPath(workspace: Workspace, session: Session, note: SessionNote, revision: number) {
    return `${sessionNotePath(workspace, session, note)}/revisions/${encodeURIComponent(revision)}`
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

  function projectNotesAPIPath(workspace: Workspace, project: Project) {
    return `/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects/${encodeURIComponent(project.id)}/notes`
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
    noteRevisionSummaries = []
    noteHistoryLoading = false
    noteRevisionLoading = false
    noteHistoryError = ""
  }

  function resetNoteHistory() {
    clearNoteHistoryPicker()
    selectedNoteRevision = null
    selectedNoteRevisionNumber = null
  }

  function openNoteHistory(notesPath: string, noteID: string) {
    if (!noteHistoryDialogElement?.open) {
      noteHistoryDialogElement?.showModal()
    }
    void loadNoteHistory(notesPath, noteID)
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
    agents = []
    selectedAgent = ""
    latestSessions = []
    activeSession = null
    activeSessionNote = null
    creatingSessionNote = false
    editingSessionNote = false
    sessionNotes = []
    sessionSecretBreadcrumb = null
    activeProject = null
    activeProjectNote = null
    creatingProjectNote = false
    projectSecretBreadcrumb = null
    projectFiles = []
    projectFileError = ""
    projectNotes = []
		projectSecrets = []
    events = []
    showJumpToLatest = false
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
      await initializeRouteProjection(generation, () => loadInitialSessionProjection(workspace, session))
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

  async function loadInitialSessionProjection(workspace: Workspace, session: Session) {
    if (isSessionNotesRoute()) {
      await loadSessionNotes(session)
      const noteID = routeSessionNoteID(route)
      if (noteID !== null && noteID !== "new") {
        const note = await loadSessionNote(session, noteID)
        if (note !== null && route.kind === "session-note-edit") {
          activateSessionNoteEdit()
        }
        if (note !== null && route.kind === "session-note-revision") {
          await loadSessionNoteRevision(session, noteID, route.revision)
        }
      }
    } else if (!isSessionSecretsRoute()) {
      await Promise.all([loadSessionEvents(session), refreshWorkspaceAgents(workspace)])
    }
  }

  async function loadInitialProjectProjection(workspace: Workspace, project: Project) {
    await Promise.all([loadProjectSessions(project), loadProjectFiles(project), loadProjectNotes(project), loadProjectSecrets(project)])
    const noteID = routeProjectNoteID(route)
    if (noteID !== null && noteID !== "new") {
      const note = await loadProjectNote(project, noteID)
      if (note === null) {
        navigate(`${projectsPath(workspace)}/${encodeURIComponent(project.id)}`, true)
      } else {
        activeProjectNote = note
      }
    }
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
    agents = []
    selectedAgent = ""
    latestSessions = []
    activeSession = null
    activeSessionNote = null
    creatingSessionNote = false
    editingSessionNote = false
    sessionNotes = []
    sessionSecretBreadcrumb = null
    activeProject = null
    activeProjectNote = null
    creatingProjectNote = false
    projectSecretBreadcrumb = null
    projectFiles = []
    projectFileError = ""
    projectNotes = []
		projectSecrets = []
    events = []
    showJumpToLatest = false
    workspaceContentStatus = "checking"
    return !signal.aborted && isCurrentRoute(generation)
  }

  function activateWorkspacePage() {
    stopActivityPolling()
    activeSession = null
    activeProject = null
    activeProjectNote = null
    activeSessionNote = null
    creatingProjectNote = false
    creatingSessionNote = false
    events = []
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
      showJumpToLatest = false
    }
    resetNoteHistory()
    activeSessionNote = null
    creatingSessionNote = false
    editingSessionNote = false
    sessionNotes = []
    sessionSecretBreadcrumb = null
    activeProjectNote = null
    creatingProjectNote = false
    projectFiles = []
    projectNotes = []
		projectSecrets = []
    if (isSessionNotesRoute()) {
      initializeSessionNotesRoute()
    } else if (!isSessionSecretsRoute()) {
      events = []
      eventStatus = "checking"
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
    resetNoteHistory()
    activeProjectNote = null
    creatingProjectNote = false
    projectSecretBreadcrumb = null
    projectFiles = []
    projectFileError = ""
    projectNotes = []
		projectSecrets = []
    events = []
    const noteID = routeProjectNoteID(route)
    if (noteID === "new") {
      activateProjectNoteCreate()
    }
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

  async function loadProjectNote(project: Project, id: string) {
    if (activeWorkspace === null) {
      return null
    }
    const workspace = activeWorkspace
    const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects/${encodeURIComponent(project.id)}/notes/${encodeURIComponent(id)}`, { credentials: "same-origin" })
    if (response.status === 401) {
      signInRequired()
      return null
    }
    if (response.status === 404) {
      return null
    }
    if (!response.ok) {
      throw new Error("project note could not be loaded")
    }
    return (await response.json()) as ProjectNote
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

  async function loadSessionNotes(session: Session, showLoading = true) {
    if (activeWorkspace === null || !isSessionNotesRoute()) {
      return false
    }
    const workspace = activeWorkspace
    const generation = ++sessionNotesGeneration
    if (showLoading) {
      sessionNoteStatus = "checking"
    }
    try {
      const response = await fetch(sessionNotesAPIPath(workspace, session), { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return false
      }
      if (!response.ok) {
        throw new Error("session notes could not be loaded")
      }
      const loaded = (await response.json()) as SessionNote[]
      if (generation !== sessionNotesGeneration || activeWorkspace?.id !== workspace.id || activeSession?.id !== session.id || !isSessionNotesRoute()) {
        return false
      }
      sessionNotes = [...loaded].sort((left, right) => {
        const difference = new Date(right.created_at).getTime() - new Date(left.created_at).getTime()
        return Number.isFinite(difference) && difference !== 0 ? difference : right.id.localeCompare(left.id)
      })
      sessionNoteStatus = "ready"
      return true
    } catch {
      if (generation === sessionNotesGeneration && activeWorkspace?.id === workspace.id && activeSession?.id === session.id && isSessionNotesRoute()) {
        sessionNoteStatus = "unavailable"
      }
      return false
    }
  }

  async function loadSessionNote(session: Session, id: string) {
    if (activeWorkspace === null || routeSessionNoteID(route) !== id) {
      return null
    }
    const workspace = activeWorkspace
    const generation = ++sessionNoteGeneration
    sessionNotePageStatus = "checking"
    try {
      const response = await fetch(`${sessionNotesAPIPath(workspace, session)}/${encodeURIComponent(id)}`, { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return null
      }
      if (response.status === 404) {
        if (generation === sessionNoteGeneration && activeWorkspace?.id === workspace.id && activeSession?.id === session.id && routeSessionNoteID(route) === id) {
          sessionNotePageStatus = "not-found"
        }
        return null
      }
      if (!response.ok) {
        throw new Error("session note could not be loaded")
      }
      const loaded = (await response.json()) as SessionNote
      if (generation !== sessionNoteGeneration || activeWorkspace?.id !== workspace.id || activeSession?.id !== session.id || routeSessionNoteID(route) !== id) {
        return null
      }
      activeSessionNote = loaded
      sessionNotePageStatus = "ready"
      return loaded
    } catch {
      if (generation === sessionNoteGeneration && activeWorkspace?.id === workspace.id && activeSession?.id === session.id && routeSessionNoteID(route) === id) {
        sessionNotePageStatus = "unavailable"
      }
      return null
    }
  }

  async function loadSessionNoteRevision(session: Session, noteID: string, revision: number) {
    if (activeWorkspace === null || routeSessionNoteID(route) !== noteID || routeSessionNoteRevision(route) !== revision) {
      return null
    }
    const workspace = activeWorkspace
    const generation = ++sessionNoteRevisionGeneration
    sessionNotePageStatus = "checking"
    try {
      const response = await fetch(`${sessionNotesAPIPath(workspace, session)}/${encodeURIComponent(noteID)}/revisions/${encodeURIComponent(revision)}`, { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return null
      }
      if (response.status === 404) {
        if (generation === sessionNoteRevisionGeneration && activeWorkspace?.id === workspace.id && activeSession?.id === session.id && routeSessionNoteID(route) === noteID && routeSessionNoteRevision(route) === revision) {
          sessionNotePageStatus = "not-found"
        }
        return null
      }
      if (!response.ok) {
        throw new Error("session note revision could not be loaded")
      }
      const loaded = (await response.json()) as NoteRevision
      if (generation !== sessionNoteRevisionGeneration || activeWorkspace?.id !== workspace.id || activeSession?.id !== session.id || routeSessionNoteID(route) !== noteID || routeSessionNoteRevision(route) !== revision) {
        return null
      }
      selectedNoteRevision = loaded
      selectedNoteRevisionNumber = revision
      sessionNotePageStatus = "ready"
      noteHistoryDialogElement?.close()
      return loaded
    } catch {
      if (generation === sessionNoteRevisionGeneration && activeWorkspace?.id === workspace.id && activeSession?.id === session.id && routeSessionNoteID(route) === noteID && routeSessionNoteRevision(route) === revision) {
        sessionNotePageStatus = "unavailable"
      }
      return null
    }
  }

  function initializeSessionNotesRoute() {
    if (route.kind === "session-note-new" && creatingSessionNote && editingSessionNote && activeSessionNote === null) {
      return
    }
    sessionNotesGeneration += 1
    sessionNoteGeneration += 1
    sessionNoteRevisionGeneration += 1
    resetNoteHistory()
    activeSessionNote = null
    creatingSessionNote = false
    editingSessionNote = false
    sessionNotes = []
    sessionNoteStatus = "checking"
    sessionNotePageStatus = "checking"
    if (route.kind === "session-note-new") {
      activateSessionNoteCreate()
      sessionNotePageStatus = "ready"
    }
  }

  async function loadSessionEvents(session: Session, showLoading = true, projection?: ActivityProjection) {
    if (activeWorkspace === null) {
      return false
    }
    if (showLoading) {
      eventStatus = "checking"
    }
    try {
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(activeWorkspace.id)}/sessions/${encodeURIComponent(session.id)}/events?limit=100`, { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return false
      }
      if (!response.ok) {
        throw new Error("session events could not be loaded")
      }
      const loaded = (await response.json()) as SessionEventTree[]
      if (projection !== undefined && !isActiveProjection(projection) || activeSession?.id !== session.id || route.kind !== "session-chat") {
        return false
      }
      const knownEvents = new Set(events.flatMap(eventTreeIDs))
      const hasNewEvents = loaded.some((tree) => eventTreeIDs(tree).some((id) => !knownEvents.has(id)))
      const shouldFollow = showLoading || isNearChatBottom()
      events = loaded
      eventStatus = "ready"
      if (showLoading || hasNewEvents) {
        if (shouldFollow) {
          void scrollToLatest(showLoading ? "instant" : "smooth")
        } else {
          showJumpToLatest = true
        }
      }
      const finishedReplies = new Set(loaded.filter((tree) => finalReplies(tree).length > 0 || hasThinkingFailure(tree) || hasCancellationSuccess(tree)).map((tree) => tree.event.ref.id))
      if (awaitingReplyFor.length > 0 && awaitingReplyFor.some((eventID) => finishedReplies.has(eventID))) {
        awaitingReplyFor = awaitingReplyFor.filter((eventID) => !finishedReplies.has(eventID))
      }
      return true
    } catch {
      if ((projection === undefined || isActiveProjection(projection)) && activeSession?.id === session.id && route.kind === "session-chat") {
        eventStatus = "unavailable"
      }
      return false
    }
  }

  function stopActivityPolling() {
    disposeActiveProjection()
    awaitingReplyFor = []
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

  async function refreshWorkspaceAgents(workspace: Workspace, projection?: ActivityProjection) {
    const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/agents`, { credentials: "same-origin" })
    if (response.status === 401) {
      signInRequired()
      return false
    }
    if (!response.ok) {
      throw new Error("agents could not be refreshed")
    }
    const loaded = (await response.json()) as WorkspaceAgent[]
    if ((projection !== undefined && !isActiveProjection(projection)) || activeWorkspace?.id !== workspace.id || activeSession === null) {
      return false
    }
    agents = loaded
    if (!agents.some((agent) => agent.id === selectedAgent)) {
      selectedAgent = ""
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
      events: ["session.*", "session_event.*", "session_file.*", "session_note.*", ...(isSessionSecretsRoute() ? [] : ["session_secret.*"])],
    } satisfies ActivitySelector
    const projectID = session?.project?.id ?? activeProject?.id
    const projectSelector = projectID === undefined ? (overview ? {
      name: "project",
      topic: workspace.id,
      events: ["project.*"],
    } satisfies ActivitySelector : undefined) : {
      name: "project",
      topic: `${workspace.id}/${projectID}`,
      events: ["project.*", "project_file.*", "project_note.*", ...(isProjectSecretsRoute() ? [] : ["project_secret.*"]), "session.*"],
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
      const agentChanged = agentSelector !== undefined && names.has(agentSelector.name)
      const sessionChanged = sessionSelector !== undefined && names.has(sessionSelector.name)
      const projectChanged = projectSelector !== undefined && names.has(projectSelector.name)

      let refreshed = (await Promise.all([
        ...(workspaceChanged ? [loadWorkspaces(false)] : []),
        ...(sessionSelector !== undefined ? [refreshWorkspaceSessions(workspace, projection)] : []),
        ...(projectSelector !== undefined ? [refreshWorkspaceProjects(workspace, projection)] : []),
        ...(agentChanged ? [refreshWorkspaceAgents(workspace, projection)] : []),
      ])).every(Boolean)
      if (!isActiveProjection(projection) || signal.aborted) {
        throw new Error("activity projection refresh failed")
      }
      if (session !== null && (sessionChanged || projectChanged)) {
        if (isSessionNotesRoute()) {
          if (sessionChanged) {
            const notesLoaded = await loadSessionNotes(session, false)
            const noteID = routeSessionNoteID(route)
            let noteLoaded = true
            if (noteID !== null && noteID !== "new") {
              const loaded = await loadSessionNote(session, noteID)
              noteLoaded = loaded !== null || sessionNotePageStatus !== "unavailable"
              if (loaded !== null && route.kind === "session-note-edit" && !editingSessionNote) {
                activateSessionNoteEdit()
              }
              if (loaded !== null && route.kind === "session-note-revision") {
                noteLoaded = (await loadSessionNoteRevision(session, noteID, route.revision)) !== null || sessionNotePageStatus !== "unavailable"
              }
            }
            refreshed = notesLoaded && noteLoaded
          }
        } else if (!isSessionSecretsRoute()) {
          refreshed = await loadSessionEvents(session, false, projection)
        }
      }
      if (refreshed && session === null && projectChanged && activeProject !== null && activeProject.id === projectID) {
		refreshed = (await Promise.all([loadProjectSessions(activeProject, projection), loadProjectFiles(activeProject, false, projection), loadProjectNotes(activeProject, false, projection), loadProjectSecrets(activeProject, false, projection)])).every(Boolean)
        const noteID = routeProjectNoteID(route)
        if (refreshed && noteID !== null && noteID !== "new") {
          const loaded = await loadProjectNote(activeProject, noteID)
          if (!isActiveProjection(projection)) {
            throw new Error("activity projection refresh failed")
          }
          if (loaded !== null) {
            if (activeProjectNote?.id === noteID && loaded.revision !== activeProjectNote.revision) {
              resetNoteHistory()
            }
            activeProjectNote = loaded
          } else {
            activeProjectNote = null
            editingProjectNote = false
            navigate(`${projectsPath(workspace)}/${encodeURIComponent(activeProject.id)}`, true)
          }
        }
      }
      if (!refreshed || signal.aborted || !isActiveProjection(projection)) {
        throw new Error("activity projection refresh failed")
      }
    })
  }

  function eventTreeIDs(tree: SessionEventTree): string[] {
    return [tree.event.ref.id, ...tree.children.flatMap(eventTreeIDs)]
  }

  function finalReplies(tree: SessionEventTree) {
    return tree.children.filter((child) => child.event.kind === "message.text" && child.event.author_agent !== undefined && child.event.payload.text !== undefined)
  }

  function activityEvents(tree: SessionEventTree) {
    return tree.children.filter((child) => !finalReplies(tree).includes(child))
  }

  function renderedActivityEvents(tree: SessionEventTree) {
    return activityEvents(tree).filter((activity) => activity.event.kind === "tool.request" || activity.event.kind === "thinking.started")
  }

  function displayedActivityEvents(tree: SessionEventTree) {
    const activity = renderedActivityEvents(tree)
    if (expandedActivity.has(tree.event.ref.id) || activity.length <= 5) {
      return activity
    }
    return activity.slice(-5)
  }

  function toggleActivity(tree: SessionEventTree) {
    const next = new Set(expandedActivity)
    if (next.has(tree.event.ref.id)) {
      next.delete(tree.event.ref.id)
    } else {
      next.add(tree.event.ref.id)
    }
    expandedActivity = next
  }

  function replyDuration(tree: SessionEventTree) {
    const replies = finalReplies(tree)
    if (replies.length === 0) {
      return ""
    }
    return elapsedDuration(tree.event.created_at, replies[replies.length - 1].event.created_at)
  }

  function activityAgentLabel(tree: SessionEventTree) {
    if (tree.event.payload.agent !== undefined) {
      return agentLabel(tree.event.payload.agent)
    }
    const activity = activityEvents(tree).find((child) => child.event.author_agent !== undefined)
    if (activity?.event.author_agent !== undefined) {
      return agentLabel(activity.event.author_agent.model.id)
    }
    const reply = finalReplies(tree)[0]
    return reply?.event.author_agent === undefined ? "Agent" : agentLabel(reply.event.author_agent.model.id)
  }

  function workingReplyDuration(tree: SessionEventTree) {
    return elapsedDuration(tree.event.created_at, activityPollTimestamp)
  }

  function toolStatus(tree: SessionEventTree) {
    return activityStatus(tree, "tool.success", "tool.failure")
  }

  function thinkingStatus(tree: SessionEventTree) {
    return activityStatus(tree, "thinking.completed", "thinking.failed")
  }

  function activityStatus(tree: SessionEventTree, completedKind: string, failedKind: string) {
    if (tree.children.some((child) => child.event.kind === failedKind)) {
      return "failed"
    }
    if (tree.children.some((child) => child.event.kind === completedKind)) {
      return "succeeded"
    }
    return "working"
  }

  function toolCallDuration(tree: SessionEventTree) {
    return activityDuration(tree, "tool.success", "tool.failure")
  }

  function approvalRequests(tree: SessionEventTree) {
    return tree.children.filter((child) => child.event.kind === "approval.request")
  }

  function approvalResponse(tree: SessionEventTree) {
    return tree.children.find((child) => child.event.kind === "approval.approved" || child.event.kind === "approval.rejected")
  }

  function approvalError(tree: SessionEventTree) {
    return approvalErrors.get(tree.event.ref.id) ?? ""
  }

  function approvalDescription(approval: SessionEventTree, task: SessionEventTree) {
    return approval.event.payload.description ?? task.event.payload.reason ?? `Run ${task.event.payload.name ?? "tool"}`
  }

  function thinkingDuration(tree: SessionEventTree) {
    return activityDuration(tree, "thinking.completed", "thinking.failed")
  }

  function activityDuration(tree: SessionEventTree, completedKind: string, failedKind: string) {
    const completed = tree.children.find((child) => child.event.kind === completedKind || child.event.kind === failedKind)
    if (completed === undefined) {
      return ""
    }
    return elapsedDuration(tree.event.created_at, completed.event.created_at)
  }

  function elapsedDuration(startedAt: string, completedAt: string | number) {
    const elapsed = new Date(completedAt).getTime() - new Date(startedAt).getTime()
    if (!Number.isFinite(elapsed) || elapsed < 0) {
      return ""
    }
    if (elapsed < 100) {
      return "<0.1s"
    }
    if (elapsed >= 60_000) {
      const seconds = Math.floor(elapsed / 1000)
      return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
    }
    return `${(elapsed / 1000).toFixed(1)}s`
  }

  function hasThinkingFailure(tree: SessionEventTree) {
    return tree.children.some((child) => child.event.kind === "thinking.started" && thinkingStatus(child) === "failed")
  }

  function cancellationRequest(tree: SessionEventTree) {
    return tree.children.find((child) => child.event.kind === "cancel.request")
  }

  function hasCancellationSuccess(tree: SessionEventTree) {
    const request = cancellationRequest(tree)
    return request?.children.some((child) => child.event.kind === "cancel.success") ?? false
  }

  function replyCanBeCancelled(tree: SessionEventTree) {
    return finalReplies(tree).length === 0 && !hasThinkingFailure(tree) && cancellationRequest(tree) === undefined
  }

  async function cancelReply(tree: SessionEventTree) {
    if (activeWorkspace === null || activeSession === null || cancellingReplyFor.has(tree.event.ref.id)) {
      return
    }
    const next = new Set(cancellingReplyFor)
    next.add(tree.event.ref.id)
    cancellingReplyFor = next
    try {
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(activeWorkspace.id)}/sessions/${encodeURIComponent(activeSession.id)}/messages/${encodeURIComponent(tree.event.ref.id)}/cancel`, {
        method: "POST",
        credentials: "same-origin",
      })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("reply cancellation failed")
      }
      await loadSessionEvents(activeSession, false)
    } catch {
      messageError = "The reply could not be cancelled. Try again."
    } finally {
      const completed = new Set(cancellingReplyFor)
      completed.delete(tree.event.ref.id)
      cancellingReplyFor = completed
    }
  }

  async function respondToApproval(approval: SessionEventTree, decision: "approved" | "rejected") {
    if (activeWorkspace === null || activeSession === null || submittingApprovals.has(approval.event.ref.id)) {
      return
    }
    const workspace = activeWorkspace
    const session = activeSession
    const submitting = new Set(submittingApprovals)
    submitting.add(approval.event.ref.id)
    submittingApprovals = submitting
    const clearedErrors = new Map(approvalErrors)
    clearedErrors.delete(approval.event.ref.id)
    approvalErrors = clearedErrors
    try {
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions/${encodeURIComponent(session.id)}/approvals/${encodeURIComponent(approval.event.ref.id)}`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ decision }),
      })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (response.status === 409) {
        throw new Error("This approval has already been decided.")
      }
      if (!response.ok) {
        throw new Error("The approval response could not be submitted. Try again.")
      }
      await loadSessionEvents(session, false)
    } catch (error) {
      const nextErrors = new Map(approvalErrors)
      nextErrors.set(approval.event.ref.id, error instanceof Error ? error.message : "The approval response could not be submitted. Try again.")
      approvalErrors = nextErrors
    } finally {
      const completed = new Set(submittingApprovals)
      completed.delete(approval.event.ref.id)
      submittingApprovals = completed
    }
  }

  async function copyMarkdown(text: string) {
    await navigator.clipboard.writeText(text)
  }

  function isNearChatBottom() {
    if (workspaceMainElement === undefined) {
      return true
    }
    return workspaceMainElement.scrollHeight - workspaceMainElement.scrollTop - workspaceMainElement.clientHeight < 64
  }

  async function scrollToLatest(behavior: ScrollBehavior = "smooth") {
    await tick()
    if (workspaceMainElement === undefined) {
      return
    }
    workspaceMainElement.scrollTo({ top: workspaceMainElement.scrollHeight, behavior })
    showJumpToLatest = false
  }

  function trackChatScroll() {
    if (isNearChatBottom()) {
      showJumpToLatest = false
    }
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

  function activateProjectNoteCreate() {
    if (activeWorkspace === null || activeProject === null) {
      return
    }
    resetNoteHistory()
    activeProjectNote = null
    creatingProjectNote = true
    editingProjectNote = true
    projectNoteTitle = ""
    projectNoteDescription = ""
    projectNoteBody = ""
    projectNoteSensitive = false
    projectNoteError = ""
  }

  function startProjectNoteCreate() {
    if (activeWorkspace !== null && activeProject !== null) {
      navigate(projectNotePath(activeWorkspace, activeProject, "new"))
    }
  }

  function startProjectNoteEdit() {
    if (activeProjectNote === null) {
      return
    }
    projectNoteTitle = activeProjectNote.title
    projectNoteDescription = activeProjectNote.description
    projectNoteBody = activeProjectNote.body ?? ""
    projectNoteSensitive = activeProjectNote.sensitive
    projectNoteError = ""
    editingProjectNote = true
  }

  function cancelProjectNoteEdit() {
    if (savingProjectNote || activeWorkspace === null || activeProject === null) {
      return
    }
    projectNoteError = ""
    if (creatingProjectNote) {
      creatingProjectNote = false
      editingProjectNote = false
      navigate(`${projectsPath(activeWorkspace)}/${encodeURIComponent(activeProject.id)}`)
      return
    }
    editingProjectNote = false
  }

  async function saveProjectNote() {
    if (activeWorkspace === null || activeProject === null || projectNoteTitle.trim() === "") {
      projectNoteError = "Title is required."
      return
    }
    const workspace = activeWorkspace
    const project = activeProject
    const creating = creatingProjectNote
    const note = activeProjectNote
    projectNoteError = ""
    savingProjectNote = true
    try {
      const notesPath = projectNotesAPIPath(workspace, project)
      const path = creating ? notesPath : `${notesPath}/${encodeURIComponent(note?.id ?? "")}`
      const response = await fetch(path, {
        method: creating ? "POST" : "PATCH",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ title: projectNoteTitle, description: projectNoteDescription, body: projectNoteBody, sensitive: projectNoteSensitive }),
      })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("project note could not be saved")
      }
      const saved = (await response.json()) as ProjectNote
      if (activeWorkspace?.id !== workspace.id || activeProject?.id !== project.id) {
        return
      }
      activeProjectNote = saved
      resetNoteHistory()
      creatingProjectNote = false
      editingProjectNote = false
      projectNotes = [saved, ...projectNotes.filter((candidate) => candidate.id !== saved.id)]
      navigate(projectNotePath(workspace, project, saved))
    } catch {
      projectNoteError = "The note could not be saved. Try again."
    } finally {
      savingProjectNote = false
    }
  }

  async function removeProjectNote() {
    if (activeWorkspace === null || activeProject === null || activeProjectNote === null || deletingProjectNote || !window.confirm(`Remove ${activeProjectNote.title}?`)) {
      return
    }
    const workspace = activeWorkspace
    const project = activeProject
    const note = activeProjectNote
    deletingProjectNote = true
    projectNoteError = ""
    try {
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects/${encodeURIComponent(project.id)}/notes/${encodeURIComponent(note.id)}`, { method: "DELETE", credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("project note could not be removed")
      }
      if (activeWorkspace?.id === workspace.id && activeProject?.id === project.id && activeProjectNote?.id === note.id) {
        resetNoteHistory()
        activeProjectNote = null
        editingProjectNote = false
        projectNotes = projectNotes.filter((candidate) => candidate.id !== note.id)
        navigate(`${projectsPath(workspace)}/${encodeURIComponent(project.id)}`)
      }
    } catch {
      projectNoteError = "The note could not be removed. Try again."
    } finally {
      deletingProjectNote = false
    }
  }

  function activateSessionNoteCreate() {
    if (activeWorkspace === null || activeSession === null) {
      return
    }
    resetNoteHistory()
    activeSessionNote = null
    creatingSessionNote = true
    editingSessionNote = true
    sessionNoteTitle = ""
    sessionNoteDescription = ""
    sessionNoteBody = ""
    sessionNoteSensitive = false
    sessionNoteError = ""
  }

  function startSessionNoteCreate() {
    if (activeWorkspace !== null && activeSession !== null) {
      navigate(sessionNotePath(activeWorkspace, activeSession, "new"))
    }
  }

  function activateSessionNoteEdit() {
    if (activeSessionNote === null || activeWorkspace === null || activeSession === null) {
      return
    }
    sessionNoteTitle = activeSessionNote.title
    sessionNoteDescription = activeSessionNote.description
    sessionNoteBody = activeSessionNote.body ?? ""
    sessionNoteSensitive = activeSessionNote.sensitive
    sessionNoteError = ""
    editingSessionNote = true
  }

  function startSessionNoteEdit() {
    if (activeWorkspace !== null && activeSession !== null && activeSessionNote !== null) {
      navigate(sessionNoteEditPath(activeWorkspace, activeSession, activeSessionNote))
    }
  }

  function cancelSessionNoteEdit() {
    if (savingSessionNote || activeWorkspace === null || activeSession === null) {
      return
    }
    if (creatingSessionNote) {
      navigate(sessionNotesPath(activeWorkspace, activeSession))
      return
    }
    navigate(sessionNotePath(activeWorkspace, activeSession, activeSessionNote!))
  }

  async function openSessionNoteRevision(revision: number) {
    if (activeWorkspace === null || activeSession === null || activeSessionNote === null) {
      return
    }
    navigate(sessionNoteRevisionPath(activeWorkspace, activeSession, activeSessionNote, revision))
  }

  function showCurrentSessionNoteRevision() {
    if (activeWorkspace === null || activeSession === null || activeSessionNote === null) {
      return
    }
    noteHistoryDialogElement?.close()
    navigate(sessionNotePath(activeWorkspace, activeSession, activeSessionNote))
  }

  async function saveSessionNote() {
    if (activeWorkspace === null || activeSession === null || sessionNoteTitle.trim() === "") {
      sessionNoteError = "Title is required."
      return
    }
    const workspace = activeWorkspace
    const session = activeSession
    const creating = creatingSessionNote
    const note = activeSessionNote
    sessionNoteError = ""
    savingSessionNote = true
    try {
      const notesPath = sessionNotesAPIPath(workspace, session)
      const path = creating ? notesPath : `${notesPath}/${encodeURIComponent(note?.id ?? "")}`
      const response = await fetch(path, {
        method: creating ? "POST" : "PATCH",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ title: sessionNoteTitle, description: sessionNoteDescription, body: sessionNoteBody, sensitive: sessionNoteSensitive }),
      })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("session note could not be saved")
      }
      const saved = (await response.json()) as SessionNote
      if (activeWorkspace?.id !== workspace.id || activeSession?.id !== session.id) {
        return
      }
      activeSessionNote = saved
      resetNoteHistory()
      creatingSessionNote = false
      editingSessionNote = false
      sessionNotes = [saved, ...sessionNotes.filter((candidate) => candidate.id !== saved.id)]
      navigate(sessionNotePath(workspace, session, saved))
      sessionNotePageStatus = "ready"
    } catch {
      sessionNoteError = "The note could not be saved. Try again."
    } finally {
      savingSessionNote = false
    }
  }

  async function removeSessionNote() {
    if (activeWorkspace === null || activeSession === null || activeSessionNote === null || deletingSessionNote || !window.confirm(`Remove ${activeSessionNote.title}?`)) {
      return
    }
    const workspace = activeWorkspace
    const session = activeSession
    const note = activeSessionNote
    deletingSessionNote = true
    sessionNoteError = ""
    try {
      const response = await fetch(`${sessionNotesAPIPath(workspace, session)}/${encodeURIComponent(note.id)}`, { method: "DELETE", credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("session note could not be removed")
      }
      if (activeWorkspace?.id === workspace.id && activeSession?.id === session.id && activeSessionNote?.id === note.id) {
        resetNoteHistory()
        activeSessionNote = null
        editingSessionNote = false
        sessionNotes = sessionNotes.filter((candidate) => candidate.id !== note.id)
        navigate(sessionNotesPath(workspace, session))
      }
    } catch {
      sessionNoteError = "The note could not be removed. Try again."
    } finally {
      deletingSessionNote = false
    }
  }

  function sessionNotesAPIPath(workspace: Workspace, session: Session) {
    return `/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions/${encodeURIComponent(session.id)}/notes`
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

  async function sendMessage() {
    if (activeWorkspace === null || activeSession === null || (messageText.trim() === "" && composerFiles.length === 0)) {
      return
    }
    const workspace = activeWorkspace
    const session = activeSession
    messageError = ""
    sendingMessage = true
    try {
      const fileIDs = await Promise.all(composerFiles.map((entry) => uploadComposerFile(workspace, session, entry)))
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions/${encodeURIComponent(session.id)}/messages`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ...(messageText.trim() === "" ? {} : { text: messageText }), ...(selectedAgent === "" ? {} : { agent: selectedAgent }), ...(fileIDs.length === 0 ? {} : { attachments: fileIDs }) }),
      })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("message could not be sent")
      }
      const event = (await response.json()) as SessionEvent
      messageText = ""
      composerFiles = []
      await tick()
      resizeMessageInput()
      if (activeSession?.id !== session.id) {
        return
      }
      events = [...events, { event, children: [] }]
      void scrollToLatest()
      awaitingReplyFor = [...awaitingReplyFor, event.ref.id]
    } catch {
      messageError = "Your message or file upload could not be sent. Try again."
    } finally {
      sendingMessage = false
      await tick()
      messageInputElement?.focus()
    }
  }

  async function uploadComposerFile(workspace: Workspace, session: Session, entry: ComposerFile) {
    if (entry.id !== undefined) {
      return entry.id
    }
    updateComposerFile(entry.file, { status: "uploading", error: undefined })
    try {
      const created = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions/${encodeURIComponent(session.id)}/files`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: entry.file.name, ...(entry.file.type === "" ? {} : { media_type: entry.file.type }) }),
      })
      if (!created.ok) {
        throw new Error("create file failed")
      }
      const upload = (await created.json()) as { file: { ref: { id: string } }; upload_url: string }
      const put = await fetch(upload.upload_url, { method: "PUT", body: entry.file, ...(entry.file.type === "" ? {} : { headers: { "Content-Type": entry.file.type } }) })
      if (!put.ok) {
        throw new Error("upload file failed")
      }
      const finished = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions/${encodeURIComponent(session.id)}/files/${encodeURIComponent(upload.file.ref.id)}/finish`, {
        method: "POST",
        credentials: "same-origin",
      })
      if (!finished.ok) {
        throw new Error("finish file failed")
      }
      updateComposerFile(entry.file, { id: upload.file.ref.id, status: "pending", error: undefined })
      return upload.file.ref.id
    } catch {
      updateComposerFile(entry.file, { status: "failed", error: "Upload failed" })
      throw new Error("upload failed")
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

  function sessionFileDownloadPath(workspace: Workspace, session: Session, file: MessageFile) {
    return `/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions/${encodeURIComponent(session.id)}/files/${encodeURIComponent(file.id)}/download`
  }

  function updateComposerFile(file: File, update: Partial<ComposerFile>) {
    composerFiles = composerFiles.map((entry) => entry.file === file ? { ...entry, ...update } : entry)
  }

  function selectComposerFiles(input: HTMLInputElement) {
    const selected = Array.from(input.files ?? [])
    composerFiles = [...composerFiles, ...selected.map((file) => ({ file, status: "pending" as const }))]
    input.value = ""
  }

  function removeComposerFile(file: File) {
    composerFiles = composerFiles.filter((entry) => entry.file !== file)
  }

  function resizeMessageInput(input = messageInputElement) {
    if (input === undefined) {
      return
    }
    input.style.height = "auto"
    const styles = window.getComputedStyle(input)
    const maximumHeight = Number.parseFloat(styles.lineHeight) * 6 + Number.parseFloat(styles.paddingTop) + Number.parseFloat(styles.paddingBottom)
    input.style.height = `${Math.min(input.scrollHeight, maximumHeight)}px`
    input.style.overflowY = input.scrollHeight > maximumHeight ? "auto" : "hidden"
  }

  function agentLabel(id: string) {
    return agents.find((agent) => agent.id === id)?.label ?? id
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

    <main class="workspace-main" bind:this={workspaceMainElement} onscroll={trackChatScroll}>
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
				{#if activeSession === null && activeProjectNote === null && !creatingProjectNote && !isProjectNotesRoute() && !isProjectSecretsRoute()}
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
                {#if activeSessionNote !== null || creatingSessionNote}
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
			{:else if activeProject !== null && (isProjectNotesRoute() || activeProjectNote !== null || creatingProjectNote)}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              {#if activeProjectNote !== null || creatingProjectNote}
                <a href={activeWorkspace !== null ? projectNotesPath(activeWorkspace, activeProject) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(projectNotesPath(activeWorkspace, activeProject)) } }}>Notes</a>
              {:else}
                <span>Notes</span>
              {/if}
              {#if activeProjectNote !== null || creatingProjectNote}
                <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
                <span class="workspace-breadcrumb-segment"><NotebookPen size={16} strokeWidth={2} aria-hidden="true" />{activeProjectNote?.title ?? "New Note"}</span>
              {/if}
            {/if}
            {#if activeSession !== null && (activeSessionNote !== null || creatingSessionNote)}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              <span class="workspace-breadcrumb-segment"><NotebookPen size={16} strokeWidth={2} aria-hidden="true" />{activeSessionNote?.title ?? "New Note"}</span>
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
		{#if activeSession === null && activeProject !== null && activeProjectNote === null && !creatingProjectNote && !isProjectSecretsRoute()}
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
      {:else if activeSession === null && activeProject !== null && isProjectSecretsRoute()}
			<ProjectSecretsPage workspace={activeWorkspace!} project={activeProject} {route} {activity} onAuthenticationLost={signInRequired} onNavigate={navigate} onBreadcrumbChange={(title) => projectSecretBreadcrumb = title} onChanged={() => void loadProjectSecrets(activeProject!, false)} />
		{:else if activeSession === null && (activeProjectNote !== null || creatingProjectNote)}
        <section class="project-note-page">
          {#if editingProjectNote}
            <form class="project-note-editor" onsubmit={(event) => { event.preventDefault(); void saveProjectNote() }}>
              <div class="project-note-page-heading"><div><p class="eyebrow">Project Note</p><h2>{creatingProjectNote ? "New Note" : "Edit Note"}</h2></div></div>
              <div class="field"><label class="label" for="project-note-title">Title</label><div class="control"><input class="input" id="project-note-title" autocomplete="off" maxlength="256" required bind:value={projectNoteTitle} /></div></div>
              <div class="field"><label class="label" for="project-note-description">Description (optional)</label><div class="control"><textarea class="textarea" id="project-note-description" autocomplete="off" rows="3" maxlength="4096" bind:value={projectNoteDescription}></textarea></div></div>
              <div class="field"><label class="label" for="project-note-body">Content (optional)</label><div class="control"><textarea class="textarea project-note-body-input" id="project-note-body" autocomplete="off" rows="18" maxlength="1048576" bind:value={projectNoteBody}></textarea></div></div>
              <div class="field"><label class="checkbox"><input type="checkbox" autocomplete="off" bind:checked={projectNoteSensitive} /> Sensitive: content is marked sensitive when agents read it.</label></div>
              {#if projectNoteError !== ""}<p class="help is-danger" aria-live="polite">{projectNoteError}</p>{/if}
              <div class="project-note-actions"><button class="button" type="button" disabled={savingProjectNote} onclick={cancelProjectNoteEdit}>Cancel</button><button class="button is-primary" type="submit" disabled={savingProjectNote}>{savingProjectNote ? "Saving..." : "Save note"}</button></div>
            </form>
          {:else if activeProjectNote !== null}
            <article class="project-note-view">
              <header class="project-note-page-heading"><div>{#if selectedNoteRevision !== null}<p class="eyebrow">Project Note Revision {selectedNoteRevision.revision}</p><h2><span class="project-note-title">{selectedNoteRevision.title}{#if selectedNoteRevision.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span></h2>{#if selectedNoteRevision.description !== ""}<p>{selectedNoteRevision.description}</p>{/if}<small>By {noteAuthorLabel(selectedNoteRevision.author)} on {createdAtLabel(selectedNoteRevision.created_at)}</small>{:else}<p class="eyebrow">Project Note</p><h2><span class="project-note-title">{activeProjectNote.title}{#if activeProjectNote.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span></h2>{#if activeProjectNote.description !== ""}<p>{activeProjectNote.description}</p>{/if}<small>By {noteAuthorLabel(activeProjectNote.author)} on {createdAtLabel(activeProjectNote.created_at)}</small>{/if}</div><div class="project-note-actions">{#if selectedNoteRevision === null}<button class="button is-small" type="button" onclick={startProjectNoteEdit}>Edit</button>{:else}<button class="button is-small" type="button" onclick={showCurrentNoteRevision}>Current revision</button>{/if}<button class="button is-small" type="button" onclick={() => openNoteHistory(projectNotesAPIPath(activeWorkspace!, activeProject!), activeProjectNote!.id)}>History</button>{#if selectedNoteRevision === null}<button class="button is-small is-danger is-light" type="button" disabled={deletingProjectNote} onclick={() => void removeProjectNote()}>{deletingProjectNote ? "Removing..." : "Remove"}</button>{/if}</div></header>
              {#if selectedNoteRevision !== null}{#if selectedNoteRevision.body !== undefined && selectedNoteRevision.body !== ""}<div class="markdown-content project-note-markdown">{@html renderMarkdown(selectedNoteRevision.body)}</div>{/if}{:else if activeProjectNote.body !== undefined && activeProjectNote.body !== ""}<div class="markdown-content project-note-markdown">{@html renderMarkdown(activeProjectNote.body)}</div>{/if}
              {#if projectNoteError !== ""}<p class="help is-danger" aria-live="polite">{projectNoteError}</p>{/if}
            </article>
          {/if}
        </section>
      {:else if activeSession !== null && isSessionNotesRoute()}
        <section class="project-note-page">
          {#if route.kind === "session-note-new" || route.kind === "session-note-edit"}
            {#if route.kind === "session-note-new" || (sessionNotePageStatus === "ready" && activeSessionNote?.id === route.noteID && editingSessionNote)}
            <form class="project-note-editor" onsubmit={(event) => { event.preventDefault(); void saveSessionNote() }}>
              <div class="project-note-page-heading"><div><p class="eyebrow">Session Note</p><h2>{creatingSessionNote ? "New Note" : "Edit Note"}</h2></div></div>
              <div class="field"><label class="label" for="session-note-title">Title</label><div class="control"><input class="input" id="session-note-title" autocomplete="off" maxlength="256" required bind:value={sessionNoteTitle} /></div></div>
              <div class="field"><label class="label" for="session-note-description">Description (optional)</label><div class="control"><textarea class="textarea" id="session-note-description" autocomplete="off" rows="3" maxlength="4096" bind:value={sessionNoteDescription}></textarea></div></div>
              <div class="field"><label class="label" for="session-note-body">Content (optional)</label><div class="control"><textarea class="textarea project-note-body-input" id="session-note-body" autocomplete="off" rows="18" maxlength="1048576" bind:value={sessionNoteBody}></textarea></div></div>
              <div class="field"><label class="checkbox"><input type="checkbox" autocomplete="off" bind:checked={sessionNoteSensitive} /> Sensitive: content is marked sensitive when agents read it.</label></div>
              {#if sessionNoteError !== ""}<p class="help is-danger" aria-live="polite">{sessionNoteError}</p>{/if}
              <div class="project-note-actions"><button class="button" type="button" disabled={savingSessionNote} onclick={cancelSessionNoteEdit}>Cancel</button><button class="button is-primary" type="submit" disabled={savingSessionNote}>{savingSessionNote ? "Saving..." : "Save note"}</button></div>
            </form>
            {:else if sessionNotePageStatus === "checking"}<p class="dashboard-empty">Loading note...</p>
            {:else if sessionNotePageStatus === "not-found"}<p class="dashboard-empty">Note not found.</p>
            {:else}<p class="dashboard-empty">Note could not be loaded.</p>
            {/if}
          {:else if route.kind === "session-note" || route.kind === "session-note-revision"}
            {#if sessionNotePageStatus === "checking"}<p class="dashboard-empty">Loading note...</p>
            {:else if sessionNotePageStatus === "not-found"}<p class="dashboard-empty">Note not found.</p>
            {:else if sessionNotePageStatus === "unavailable"}<p class="dashboard-empty">Note could not be loaded.</p>
            {:else if activeSessionNote !== null && activeSessionNote.id === route.noteID && (route.kind === "session-note" || selectedNoteRevision !== null)}
            <article class="project-note-view">
              <header class="project-note-page-heading"><div>{#if route.kind === "session-note-revision" && selectedNoteRevision !== null}<p class="eyebrow">Session Note Revision {selectedNoteRevision.revision}</p><h2><span class="project-note-title">{selectedNoteRevision.title}{#if selectedNoteRevision.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span></h2>{#if selectedNoteRevision.description !== ""}<p>{selectedNoteRevision.description}</p>{/if}<small>By {noteAuthorLabel(selectedNoteRevision.author)} on {createdAtLabel(selectedNoteRevision.created_at)}</small>{:else}<p class="eyebrow">Session Note</p><h2><span class="project-note-title">{activeSessionNote.title}{#if activeSessionNote.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span></h2>{#if activeSessionNote.description !== ""}<p>{activeSessionNote.description}</p>{/if}<small>By {noteAuthorLabel(activeSessionNote.author)} on {createdAtLabel(activeSessionNote.created_at)}</small>{/if}</div><div class="project-note-actions">{#if route.kind === "session-note-revision"}<button class="button is-small" type="button" onclick={showCurrentSessionNoteRevision}>Current revision</button>{:else}<button class="button is-small" type="button" onclick={startSessionNoteEdit}>Edit</button>{/if}<button class="button is-small" type="button" onclick={() => openNoteHistory(sessionNotesAPIPath(activeWorkspace!, activeSession!), activeSessionNote!.id)}>History</button>{#if route.kind !== "session-note-revision"}<button class="button is-small is-danger is-light" type="button" disabled={deletingSessionNote} onclick={() => void removeSessionNote()}>{deletingSessionNote ? "Removing..." : "Remove"}</button>{/if}</div></header>
              {#if route.kind === "session-note-revision" && selectedNoteRevision !== null}{#if selectedNoteRevision.body !== undefined && selectedNoteRevision.body !== ""}<div class="markdown-content project-note-markdown">{@html renderMarkdown(selectedNoteRevision.body)}</div>{/if}{:else if activeSessionNote.body !== undefined && activeSessionNote.body !== ""}<div class="markdown-content project-note-markdown">{@html renderMarkdown(activeSessionNote.body)}</div>{/if}
              {#if sessionNoteError !== ""}<p class="help is-danger" aria-live="polite">{sessionNoteError}</p>{/if}
            </article>
            {:else}<p class="dashboard-empty">Note could not be loaded.</p>
            {/if}
          {:else}
            <div class="collection-heading"><h2>Session Notes</h2><button class="button is-primary is-small" type="button" onclick={() => startSessionNoteCreate()}>New note</button></div>
            <div class="collection-list">
              {#if sessionNoteStatus === "checking"}
                <p class="dashboard-empty">Loading notes...</p>
              {:else if sessionNoteStatus === "unavailable"}
                <p class="dashboard-empty">Notes could not be loaded.</p>
              {:else}
                {#each sessionNotes as note (note.id)}
                  <a class="dashboard-row project-note-row" href={activeWorkspace !== null && activeSession !== null ? sessionNotePath(activeWorkspace, activeSession, note) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null && activeSession !== null) { navigate(sessionNotePath(activeWorkspace, activeSession, note)) } }}><span class="dashboard-row-content"><span class="project-note-title">{note.title}{#if note.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span>{#if note.description !== ""}<span class="project-note-description">{note.description}</span>{/if}<span class="dashboard-row-meta"><span>{noteAuthorLabel(note.author)}</span><time datetime={note.created_at}>{createdAtLabel(note.created_at)}</time></span></span></a>
                {:else}<p class="dashboard-empty">No notes yet.</p>{/each}
              {/if}
            </div>
          {/if}
        </section>
      {:else if activeSession !== null && isSessionSecretsRoute()}
        <SessionSecretsPage workspace={activeWorkspace!} session={activeSession} {route} {activity} onAuthenticationLost={signInRequired} onNavigate={navigate} onBreadcrumbChange={(title) => sessionSecretBreadcrumb = title} />
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
      {:else}
        <section class="chat-pane">
          <div class="chat-events" aria-live="polite">
            {#if eventStatus === "checking"}
              <p class="chat-status">Loading chat...</p>
            {:else if eventStatus === "unavailable"}
              <p class="chat-status">This chat could not be loaded.</p>
            {:else if events.length === 0}
              <p class="chat-status">Send the first message to begin.</p>
            {:else}
              {#each events as tree (tree.event.ref.id)}
                {#if tree.event.kind === "message.text" && (tree.event.payload.text !== undefined || (tree.event.payload.attachments !== undefined && tree.event.payload.attachments.length > 0))}
                  <article class="chat-message message-own">
                    <p class="chat-message-author">{tree.event.author_principal?.name ?? "User"}</p>
                    {#if tree.event.payload.text !== undefined}
                      <button class="chat-message-copy" type="button" aria-label="Copy message Markdown" title="Copy Markdown" onclick={() => void copyMarkdown(tree.event.payload.text)}>
                        <Copy size={16} strokeWidth={2} />
                      </button>
                      <div class="markdown-content chat-message-text">{@html renderMarkdown(tree.event.payload.text)}</div>
                    {/if}
                    {#if tree.event.payload.attachments !== undefined && tree.event.payload.attachments.length > 0}
                        <div class="message-files" aria-label="Attached files">
                          {#each tree.event.payload.attachments as file (file.id)}
                          <a class="message-file" href={activeWorkspace !== null && activeSession !== null ? sessionFileDownloadPath(activeWorkspace, activeSession, file) : "#"} target="_blank" rel="noopener noreferrer" download={file.name} title={file.fingerprint}>
                            <Paperclip size={14} strokeWidth={2} aria-hidden="true" />
                            <span>{file.name}</span>
                            <small>{file.size} bytes{file.media_type === undefined ? "" : ` · ${file.media_type}`}</small>
                          </a>
                        {/each}
                      </div>
                    {/if}
                  </article>
                  {#if activityEvents(tree).length > 0 || awaitingReplyFor.includes(tree.event.ref.id) || replyCanBeCancelled(tree)}
                    <section class="agent-activity-section">
                      <p class="agent-activity-heading">
                        {hasCancellationSuccess(tree) ? "Cancelled" : cancellationRequest(tree) !== undefined ? "Cancellation requested" : finalReplies(tree).length === 0 ? `${activityAgentLabel(tree)} is working` : activityAgentLabel(tree)}
                        {#if finalReplies(tree).length > 0 && replyDuration(tree) !== ""}
                          <span class="agent-activity-duration">{replyDuration(tree)}</span>
                        {/if}
                        {#if replyCanBeCancelled(tree) && workingReplyDuration(tree) !== ""}
                          <span class="agent-activity-duration">{workingReplyDuration(tree)}</span>
                        {/if}
                        {#if replyCanBeCancelled(tree)}
                          <button class="agent-activity-cancel" type="button" disabled={cancellingReplyFor.has(tree.event.ref.id)} onclick={() => void cancelReply(tree)}>{cancellingReplyFor.has(tree.event.ref.id) ? "Cancelling..." : "Cancel"}</button>
                        {/if}
                      </p>
                      {#if renderedActivityEvents(tree).length > 0}
                        <div class="agent-activity">
                          {#if renderedActivityEvents(tree).length > 5 && !expandedActivity.has(tree.event.ref.id)}
                            <p class="agent-activity-overflow">
                              <span>({renderedActivityEvents(tree).length - 5} more)</span>
                              <button type="button" onclick={() => toggleActivity(tree)}>Show all</button>
                            </p>
                          {/if}
                          {#each displayedActivityEvents(tree) as activity (activity.event.ref.id)}
                            {#if activity.event.kind === "tool.request"}
                              <p class:tool-call-failed={toolStatus(activity) === "failed"} class:tool-call-succeeded={toolStatus(activity) === "succeeded"} class="tool-call" title={activity.event.payload.name ?? "tool"}>
                                {#if toolStatus(activity) === "working"}
                                  <span class="tool-status tool-status-working" aria-hidden="true"></span>
                                {:else if toolStatus(activity) === "succeeded"}
                                  <CircleCheck class="tool-status" size={14} strokeWidth={2} aria-hidden="true" />
                                {:else}
                                  <CircleX class="tool-status" size={14} strokeWidth={2} aria-hidden="true" />
                                {/if}
                                Task: {activity.event.payload.reason ?? `Running ${activity.event.payload.name ?? "tool"}`}
                                {#if toolCallDuration(activity) !== ""}
                                  <span class="tool-call-duration">{toolCallDuration(activity)}</span>
                                {/if}
                              </p>
                              {#each approvalRequests(activity) as approval (approval.event.ref.id)}
                                {@const response = approvalResponse(approval)}
                                <section class:approval-request-resolved={response !== undefined} class="approval-request">
                                  {#if response === undefined}
                                    <ShieldQuestionMark class="approval-request-icon" size={15} strokeWidth={2} aria-hidden="true" />
                                    <span class="approval-request-heading">Action approval required:</span>
                                    <span class="approval-request-detail">{approvalDescription(approval, activity)}</span>
                                    <span class="approval-request-actions">
                                      <button class="approval-approve" type="button" disabled={submittingApprovals.has(approval.event.ref.id)} onclick={() => void respondToApproval(approval, "approved")}>{submittingApprovals.has(approval.event.ref.id) ? "Submitting..." : "Approve"}</button>
                                      <button class="approval-reject" type="button" disabled={submittingApprovals.has(approval.event.ref.id)} onclick={() => void respondToApproval(approval, "rejected")}>Reject</button>
                                    </span>
                                  {:else}
                                    {#if response.event.kind === "approval.approved"}
                                      <ShieldCheck class="approval-request-icon approval-request-approved" size={15} strokeWidth={2} aria-hidden="true" />
                                      <span class="approval-request-heading">Action approved:</span>
                                      <span class="approval-request-detail">{approvalDescription(approval, activity)}</span>
                                    {:else}
                                      <ShieldX class="approval-request-icon approval-request-rejected" size={15} strokeWidth={2} aria-hidden="true" />
                                      <span class="approval-request-heading">Action rejected:</span>
                                      <span class="approval-request-detail">{approvalDescription(approval, activity)}</span>
                                    {/if}
                                  {/if}
                                </section>
                                {#if response === undefined && approvalError(approval) !== ""}<p class="approval-request-error" role="alert">{approvalError(approval)}</p>{/if}
                              {/each}
                            {:else if activity.event.kind === "thinking.started"}
                              <p class:tool-call-failed={thinkingStatus(activity) === "failed"} class:tool-call-succeeded={thinkingStatus(activity) === "succeeded"} class="tool-call">
                                {#if thinkingStatus(activity) === "working"}
                                  <span class="tool-status tool-status-working" aria-hidden="true"></span>
                                {:else if thinkingStatus(activity) === "succeeded"}
                                  <CircleCheck class="tool-status" size={14} strokeWidth={2} aria-hidden="true" />
                                {:else}
                                  <CircleX class="tool-status" size={14} strokeWidth={2} aria-hidden="true" />
                                {/if}
                                {thinkingStatus(activity) === "working" ? "Thinking" : thinkingStatus(activity) === "succeeded" ? "Thought" : "Thinking failed after"}
                                {#if thinkingDuration(activity) !== ""}
                                  <span class="tool-call-duration">{thinkingDuration(activity)}</span>
                                {/if}
                              </p>
                            {/if}
                          {/each}
                          {#if renderedActivityEvents(tree).length > 5 && expandedActivity.has(tree.event.ref.id)}
                            <p class="agent-activity-overflow">
                              <span>({renderedActivityEvents(tree).length} steps)</span>
                              <button type="button" onclick={() => toggleActivity(tree)}>Show less</button>
                            </p>
                          {/if}
                        </div>
                      {/if}
                    </section>
                  {/if}
                  {#each finalReplies(tree) as reply (reply.event.ref.id)}
                    <article class="chat-message">
                      <p class="chat-message-author">{reply.event.author_agent === undefined ? "Gatehouse" : agentLabel(reply.event.author_agent.model.id)}</p>
                      {#if reply.event.payload.text !== ""}
                        <button class="chat-message-copy" type="button" aria-label="Copy response Markdown" title="Copy Markdown" onclick={() => void copyMarkdown(reply.event.payload.text)}>
                          <Copy size={16} strokeWidth={2} />
                        </button>
                        <div class="markdown-content chat-message-text">{@html renderMarkdown(reply.event.payload.text)}</div>
                      {:else if reply.event.payload.attachments === undefined || reply.event.payload.attachments.length === 0}
                        <div class="chat-message-text"><em>No reply.</em></div>
                      {/if}
                      {#if reply.event.payload.attachments !== undefined && reply.event.payload.attachments.length > 0}
                        <div class="message-files" aria-label="Attached files">
                          {#each reply.event.payload.attachments as file (file.id)}
                            <a class="message-file" href={activeWorkspace !== null && activeSession !== null ? sessionFileDownloadPath(activeWorkspace, activeSession, file) : "#"} target="_blank" rel="noopener noreferrer" download={file.name} title={file.fingerprint}>
                              <Paperclip size={14} strokeWidth={2} aria-hidden="true" />
                              <span>{file.name}</span>
                              <small>{file.size} bytes{file.media_type === undefined ? "" : ` · ${file.media_type}`}</small>
                            </a>
                          {/each}
                        </div>
                      {/if}
                    </article>
                  {/each}
                {/if}
              {/each}
            {/if}
            {#if showJumpToLatest}
              <button class="button is-small chat-jump" type="button" onclick={() => void scrollToLatest()}>Jump to latest</button>
            {/if}
          </div>
          <form class:sending={sendingMessage} class="chat-composer" autocomplete="off" onsubmit={(event) => { event.preventDefault(); void sendMessage() }}>
            <label class="is-sr-only" for="message">Message</label>
            <input class="is-sr-only" id="files" type="file" autocomplete="off" multiple bind:this={fileInputElement} onchange={(event) => selectComposerFiles(event.currentTarget)} />
            {#if composerFiles.length > 0}
              <div class="composer-files" aria-label="Selected files">
                {#each composerFiles as entry (entry.file)}
                  <span class:failed={entry.status === "failed"} class="composer-file">
                    {#if entry.status === "uploading"}
                      <span class="composer-file-spinner" aria-hidden="true"></span>
                    {:else}
                      <Paperclip size={14} strokeWidth={2} aria-hidden="true" />
                    {/if}
                    <span>{entry.file.name}</span>
                    <small>{entry.status === "uploading" ? "Uploading" : entry.status === "failed" ? entry.error : entry.id === undefined ? `${entry.file.size} bytes` : "Ready"}</small>
                    <button type="button" aria-label={`Remove ${entry.file.name}`} disabled={sendingMessage} onclick={() => removeComposerFile(entry.file)}><X size={14} strokeWidth={2} /></button>
                  </span>
                {/each}
              </div>
            {/if}
            <div class="chat-composer-row">
              <button class="chat-composer-attach" type="button" aria-label="Attach files" title="Attach files" disabled={sendingMessage} onclick={() => fileInputElement?.click()}>
                  <Paperclip size={20} strokeWidth={2.25} aria-hidden="true" />
              </button>
              <div class:agent-selected={selectedAgent !== ""} class="chat-composer-agent" title="Select agent">
                <Bot size={20} strokeWidth={2.25} aria-hidden="true" />
                <select id="agent" aria-label="Agent" bind:value={selectedAgent}>
                  <option value="">Automatic</option>
                  {#each agents as agent}
                    <option value={agent.id}>{agent.label ?? agent.id}</option>
                  {/each}
                </select>
              </div>
              <textarea id="message" class="textarea" rows="1" autocomplete="off" placeholder="Write a message" bind:this={messageInputElement} bind:value={messageText} disabled={sendingMessage} oninput={(event) => resizeMessageInput(event.currentTarget)} onkeydown={(event) => {
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault()
                  void sendMessage()
                }
              }}></textarea>
              <button class="button is-primary chat-composer-send" type="submit" aria-label="Send message" title="Send message" disabled={sendingMessage || (messageText.trim() === "" && composerFiles.length === 0)}>
                <Send size={20} strokeWidth={2.25} aria-hidden="true" />
              </button>
            </div>
            {#if messageError !== ""}
              <p class="help is-danger" aria-live="polite">{messageError}</p>
            {/if}
          </form>
        </section>
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
        {#if activeSession !== null && activeSessionNote !== null && activeWorkspace !== null}
          {#each noteRevisionSummaries as revision (revision.revision)}
            <button class="dashboard-row project-note-row" class:is-selected={selectedNoteRevisionNumber === revision.revision} type="button" disabled={noteRevisionLoading} onclick={() => revision.revision === activeSessionNote!.revision ? showCurrentSessionNoteRevision() : void openSessionNoteRevision(revision.revision)}><span class="dashboard-row-content"><span class="project-note-title">Revision {revision.revision}: {revision.title}{#if revision.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span>{#if revision.description !== ""}<span class="project-note-description">{revision.description}</span>{/if}<span class="dashboard-row-meta"><span>{noteAuthorLabel(revision.author)}</span><time datetime={revision.created_at}>{createdAtLabel(revision.created_at)}</time></span></span></button>
          {:else}<p class="dashboard-empty">No revisions found.</p>{/each}
        {:else if activeProject !== null && activeProjectNote !== null && activeWorkspace !== null}
          {#each noteRevisionSummaries as revision (revision.revision)}
            <button class="dashboard-row project-note-row" class:is-selected={selectedNoteRevisionNumber === revision.revision} type="button" disabled={noteRevisionLoading} onclick={() => revision.revision === activeProjectNote!.revision ? showCurrentNoteRevision() : void loadNoteRevision(projectNotesAPIPath(activeWorkspace!, activeProject!), activeProjectNote!.id, revision.revision)}><span class="dashboard-row-content"><span class="project-note-title">Revision {revision.revision}: {revision.title}{#if revision.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span>{#if revision.description !== ""}<span class="project-note-description">{revision.description}</span>{/if}<span class="dashboard-row-meta"><span>{noteAuthorLabel(revision.author)}</span><time datetime={revision.created_at}>{createdAtLabel(revision.created_at)}</time></span></span></button>
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
