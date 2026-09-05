<script lang="ts">
  import { onMount, tick } from "svelte"
  import { Bot, Building, CircleCheck, CircleX, Copy, Folder, Lock, Menu, MessageSquare, NotebookPen, Paperclip, Search, Send, ShieldCheck, ShieldQuestionMark, ShieldX, X } from "@lucide/svelte"
  import type { ActivitySelector } from "./utils/activity-poller"
  import { fetchSystemGrants, fetchWorkspaces, type Workspace } from "./app/access"
  import { createActivityClient } from "./app/activity"
  import { signIn, signOut } from "./app/auth"
  import { createAuth } from "./app/auth.svelte"
  import { createRouter } from "./app/router"
  import { renderMarkdown } from "./markdown"
  import { routeProjectID, routeProjectNoteID, routeProjectSecretID, routeSessionID, routeSessionNoteID, routeSessionNoteRevision, routeSessionSecretID, routeWorkspaceID } from "./route"
  import type { Route } from "./route"

  type SystemGrant = {
    ref: { id: string }
    principal: { id: string }
    enabled: boolean
    revision: number
  }

  type SystemPrincipal = {
    id: string
    alias?: string
    name?: string
    enabled: boolean
    revision: number
    identities: { id: string; key: string; enabled: boolean; revision: number }[]
  }

  type Group = {
    id: string
    name?: string
  }

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

  type SessionSecret = {
    id: string
    description: string
    author: { id: string; name?: string }
    created_at: string
    updated_at: string
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

  type ProjectSecret = SessionSecret

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

  type WorkspaceStatus = "checking" | "ready" | "empty" | "unavailable"
  type WorkspaceContentStatus = "checking" | "ready" | "unavailable"
  type SystemAccessStatus = "checking" | "available" | "denied" | "unavailable"
  type ActivityProjection = { stop: () => void }

  const auth = createAuth()
  let workspaceStatus = $state<WorkspaceStatus>("checking")
  let workspaceContentStatus = $state<WorkspaceContentStatus>("checking")
  let systemAccess = $state<SystemAccessStatus>("checking")
  let systemGrants = $state<SystemGrant[]>([])
  let systemPrincipals = $state<SystemPrincipal[]>([])
  let systemGrantPrincipal = $state("")
  let systemGrantError = $state("")
  let systemPrincipalError = $state("")
  let creatingSystemGrant = $state(false)
  let updatingSystemGrantIDs = $state<Set<string>>(new Set())
  let updatingSystemPrincipalIDs = $state<Set<string>>(new Set())
  let route = $state<Route>({ kind: "app-home" })
  let identity = $state("")
  let password = $state("")
  let submitting = $state(false)
  let loginError = $state("")
  let workspaces = $state<Workspace[]>([])
  let activeWorkspace = $state<Workspace | null>(null)
  let groups = $state<Group[]>([])
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
	let activeProjectSecret = $state<ProjectSecret | null>(null)
	let creatingProjectSecret = $state(false)
	let editingProjectSecret = $state(false)
	let savingProjectSecret = $state(false)
	let deletingProjectSecret = $state(false)
	let projectSecretDescription = $state("")
	let projectSecretValue = $state("")
	let projectSecretError = $state("")
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
  let activeSessionSecret = $state<SessionSecret | null>(null)
  let creatingSessionSecret = $state(false)
  let editingSessionSecret = $state(false)
  let savingSessionSecret = $state(false)
  let deletingSessionSecret = $state(false)
  let sessionSecretDescription = $state("")
  let sessionSecretValue = $state("")
  let sessionSecretError = $state("")
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
  let activeProjection: ActivityProjection | null = null
  let chatSearch = $state("")
  let projectSearch = $state("")
  let groupSearch = $state("")
  let searchedSessions = $state<Session[]>([])
  let searchedProjects = $state<Project[]>([])
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
  let sessionSecrets = $state<SessionSecret[]>([])
  let sessionSecretStatus = $state<WorkspaceContentStatus>("checking")
  let sessionSearchCursor = $state<string | null>(null)
  let projectSearchCursor = $state<string | null>(null)
  let sessionSearchLoading = $state(false)
  let projectSearchLoading = $state(false)
  let sessionSearchGeneration = 0
  let projectSearchGeneration = 0
  let sessionNotesGeneration = 0
  let sessionNoteGeneration = 0
  let sessionNoteRevisionGeneration = 0
  let routeGeneration = 0
  let routeAbortController: AbortController | null = null
  let initializingAuthenticatedSession = false
  let loadingWorkspaces = false
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

  function sessionSecretPath(workspace: Workspace, session: Session, secret: SessionSecret | string) {
    const id = typeof secret === "string" ? secret : secret.id
    return `${sessionSecretsPath(workspace, session)}/${encodeURIComponent(id)}`
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

	function projectSecretPath(workspace: Workspace, project: Project, secret: ProjectSecret | string) {
		const id = typeof secret === "string" ? secret : secret.id
		return `${projectSecretsPath(workspace, project)}/${encodeURIComponent(id)}`
	}

  function groupsPath(workspace: Workspace) {
    return `${workspacePath(workspace)}/grp`
  }

  function collectionSearchName() {
    return route.kind === "session-collection" || route.kind === "project-collection" ? route.search : ""
  }

  function collectionSearchPath(path: string, name: string) {
    const query = name.trim()
    return query === "" ? path : `${path}?${new URLSearchParams({ name: query })}`
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

  async function submitSessionSearch() {
    if (activeWorkspace === null) {
      return
    }
    navigate(collectionSearchPath(sessionsPath(activeWorkspace), chatSearch))
  }

  async function submitProjectSearch() {
    if (activeWorkspace === null) {
      return
    }
    navigate(collectionSearchPath(projectsPath(activeWorkspace), projectSearch))
  }

  function ordered<T extends { id: string }>(items: T[]) {
    return [...items].sort((left, right) => right.id.localeCompare(left.id))
  }

  function matchesSearch(item: { id: string; name?: string }, search: string) {
    const query = search.trim().toLocaleLowerCase()
    return query === "" || item.id.toLocaleLowerCase().includes(query) || item.name?.toLocaleLowerCase().includes(query) === true
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
    resetNoteHistory()
    auth.clear()
    systemAccess = "checking"
    systemGrants = []
    systemPrincipals = []
    systemGrantPrincipal = ""
    systemGrantError = ""
    systemPrincipalError = ""
    creatingSystemGrant = false
    updatingSystemGrantIDs = new Set()
    updatingSystemPrincipalIDs = new Set()
    workspaces = []
    activeWorkspace = null
    groups = []
    latestProjects = []
    agents = []
    selectedAgent = ""
    latestSessions = []
    activeSession = null
    activeSessionNote = null
    creatingSessionNote = false
    editingSessionNote = false
    sessionNotes = []
    activeSessionSecret = null
    creatingSessionSecret = false
    editingSessionSecret = false
    sessionSecretValue = ""
    sessionSecretDescription = ""
    sessionSecretError = ""
    sessionSecrets = []
    activeProject = null
    activeProjectNote = null
    creatingProjectNote = false
		activeProjectSecret = null
		creatingProjectSecret = false
		editingProjectSecret = false
		projectSecretDescription = ""
		projectSecretValue = ""
		projectSecretError = ""
    projectFiles = []
    projectFileError = ""
    projectNotes = []
		projectSecrets = []
    events = []
    showJumpToLatest = false
    workspaceStatus = "checking"
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
        await loadSystemGrants()
        configureActivityPolling()
        await activity.poll()
        await loadWorkspaces()
      }
    } finally {
      initializingAuthenticatedSession = false
    }
  }

  async function loadWorkspaces(resolveAfterLoad = true) {
    if (loadingWorkspaces) {
      return false
    }
    loadingWorkspaces = true
    workspaceStatus = "checking"
    try {
      const response = await fetchWorkspaces()
      if (response.status === 401) {
        signInRequired()
        return false
      }
      if (!response.ok) {
        throw new Error(`workspace catalog returned ${response.status}`)
      }
      workspaces = (await response.json()) as Workspace[]
      if (workspaces.length === 0) {
        activeWorkspace = null
        groups = []
    latestProjects = []
        latestSessions = []
        workspaceStatus = "empty"
        if (route.kind !== "no-access" && !isSystemRoute()) {
          navigate("/app/no-access", true)
        }
        return true
      }
      const currentWorkspaceID = activeWorkspace?.id ?? routeWorkspaceID(route)
      const currentWorkspace = currentWorkspaceID === null ? undefined : workspaces.find((workspace) => workspace.id === currentWorkspaceID)
      if (currentWorkspaceID !== null && currentWorkspace === undefined) {
        activeWorkspace = null
        workspaceStatus = "ready"
        navigate(workspacePath(workspaces[0]), true)
        return true
      }
      if (currentWorkspace !== undefined) {
        activeWorkspace = currentWorkspace
      }
      workspaceStatus = "ready"
      if (resolveAfterLoad) {
        resolveRoute()
      }
      return true
    } catch {
      workspaceStatus = "unavailable"
      return false
    } finally {
      loadingWorkspaces = false
    }
  }

  async function loadSystemGrants() {
    const previousSystemAccess = systemAccess
    systemGrantError = ""
    systemAccess = "checking"
    try {
      const response = await fetchSystemGrants()
      if (response.status === 401) {
        signInRequired()
        return false
      }
      if (response.status === 403) {
        setSystemAccess("denied", previousSystemAccess)
        systemGrants = []
        return false
      }
      if (!response.ok) {
        setSystemAccess("unavailable", previousSystemAccess)
        systemGrantError = "System grants could not be loaded."
        return false
      }
      systemGrants = (await response.json()) as SystemGrant[]
      setSystemAccess("available", previousSystemAccess)
      return true
    } catch {
      setSystemAccess("unavailable", previousSystemAccess)
      systemGrantError = "System grants could not be loaded."
      return false
    }
  }

  function setSystemAccess(next: Exclude<SystemAccessStatus, "checking">, previous = systemAccess) {
    const changed = previous !== next
    systemAccess = next
    if (changed && auth.state.status === "authenticated") {
      configureActivityPolling()
    }
  }

  async function loadSystemPrincipals() {
    systemPrincipalError = ""
    try {
      const response = await fetch("/api/v1/system/principals", { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return false
      }
      if (response.status === 403) {
        setSystemAccess("denied")
        systemPrincipals = []
        return false
      }
      if (!response.ok) {
        systemPrincipalError = "Principals could not be loaded."
        return false
      }
      systemPrincipals = (await response.json()) as SystemPrincipal[]
      return true
    } catch {
      systemPrincipalError = "Principals could not be loaded."
      return false
    }
  }

  async function setSystemPrincipalEnabled(principal: SystemPrincipal, enabled: boolean) {
    if (!enabled && !window.confirm(`Disable ${principal.name ?? principal.id}?`)) {
      return
    }
    systemPrincipalError = ""
    updatingSystemPrincipalIDs = new Set(updatingSystemPrincipalIDs).add(principal.id)
    try {
      const response = await fetch(`/api/v1/system/principals/${encodeURIComponent(principal.id)}`, { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ enabled }) })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (response.status === 403) {
        setSystemAccess("denied")
        systemPrincipals = []
        return
      }
      if (!response.ok) {
        systemPrincipalError = "Principal could not be updated."
        return
      }
      const updated = await response.json() as SystemPrincipal
      systemPrincipals = systemPrincipals.map((entry) => entry.id === updated.id ? updated : entry)
      if (!updated.enabled && auth.state.claims?.principal.ref.id === updated.id) {
        signInRequired()
      }
    } catch {
      systemPrincipalError = "Principal could not be updated."
    } finally {
      const next = new Set(updatingSystemPrincipalIDs)
      next.delete(principal.id)
      updatingSystemPrincipalIDs = next
    }
  }

  async function createSystemGrant() {
    systemGrantError = ""
    const principal = systemGrantPrincipal.trim()
    if (principal === "") {
      systemGrantError = "A principal ID is required."
      return
    }
    creatingSystemGrant = true
    try {
      const response = await fetch("/api/v1/system/grants", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ principal }) })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (response.status === 403) {
        setSystemAccess("denied")
        systemGrants = []
        return
      }
      if (response.status === 404) {
        systemGrantError = "That principal does not exist."
        return
      }
      if (response.status === 409) {
        systemGrantError = "That principal already has a system grant."
        return
      }
      if (!response.ok) {
        systemGrantError = "System grant could not be created."
        return
      }
      const grant = await response.json() as SystemGrant
      systemGrants = [...systemGrants, grant].sort((left, right) => left.ref.id.localeCompare(right.ref.id))
      systemGrantPrincipal = ""
    } catch {
      systemGrantError = "System grant could not be created."
    } finally {
      creatingSystemGrant = false
    }
  }

  async function setSystemGrantEnabled(grant: SystemGrant, enabled: boolean) {
    if (!enabled && !window.confirm(`Disable system access for ${grant.principal.id}?`)) {
      return
    }
    systemGrantError = ""
    updatingSystemGrantIDs = new Set(updatingSystemGrantIDs).add(grant.ref.id)
    try {
      const response = await fetch(`/api/v1/system/grants/${encodeURIComponent(grant.ref.id)}`, { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ enabled }) })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (response.status === 403) {
        setSystemAccess("denied")
        systemGrants = []
        return
      }
      if (!response.ok) {
        systemGrantError = "System grant could not be updated."
        return
      }
      const updated = await response.json() as SystemGrant
      systemGrants = systemGrants.map((entry) => entry.ref.id === updated.ref.id ? updated : entry)
      if (!updated.enabled && auth.state.claims?.principal.ref.id === updated.principal.id) {
        setSystemAccess("denied")
        systemGrants = []
      }
    } catch {
      systemGrantError = "System grant could not be updated."
    } finally {
      const next = new Set(updatingSystemGrantIDs)
      next.delete(grant.ref.id)
      updatingSystemGrantIDs = next
    }
  }

  async function activateRoute(generation: number, signal: AbortSignal) {
    if (auth.state.status !== "authenticated") {
      void checkSession()
      return
    }
    if (isSystemRoute()) {
      await loadSystemGrants()
      if (route.kind === "system-principals") {
        await loadSystemPrincipals()
      }
      configureActivityPolling()
      return
    }
    if (workspaceStatus !== "ready") {
      if (!loadingWorkspaces) {
        void loadWorkspaces()
      }
      return
    }
    if (route.kind === "login") {
      navigate(nextPath(), true)
      return
    }
    const requestedWorkspaceID = routeWorkspaceID(route)
    const workspace = workspaces.find((candidate) => candidate.id === requestedWorkspaceID) ?? workspaces[0]
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
    if (isChatCollection()) {
      await loadSessionSearch(true)
    } else if (isProjectCollection()) {
      await loadProjectSearch(true)
    } else if (isGroupCollection()) {
      await refreshWorkspaceGroups(workspace)
    } else {
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
    } else if (isSessionSecretsRoute()) {
      await loadSessionSecrets(session)
      const secretID = routeSessionSecretID(route)
      if (secretID !== null && secretID !== "new") {
        const secret = await loadSessionSecret(session, secretID)
        if (secret === null) {
          navigate(sessionSecretsPath(workspace, session), true)
        } else {
          activeSessionSecret = secret
        }
      }
    } else {
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
    const secretID = routeProjectSecretID(route)
    if (secretID !== null && secretID !== "new") {
      const secret = await loadProjectSecret(project, secretID)
      if (secret === null) {
        navigate(projectSecretsPath(workspace, project), true)
      } else {
        activeProjectSecret = secret
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
    groups = []
    latestProjects = []
    agents = []
    selectedAgent = ""
    latestSessions = []
    activeSession = null
    activeSessionNote = null
    creatingSessionNote = false
    editingSessionNote = false
    sessionNotes = []
    activeSessionSecret = null
    creatingSessionSecret = false
    editingSessionSecret = false
    sessionSecretValue = ""
    sessionSecretDescription = ""
    sessionSecretError = ""
    sessionSecrets = []
    activeProject = null
    activeProjectNote = null
    creatingProjectNote = false
		activeProjectSecret = null
		creatingProjectSecret = false
		editingProjectSecret = false
		projectSecretDescription = ""
		projectSecretValue = ""
		projectSecretError = ""
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
    activeProjectSecret = null
    activeSessionNote = null
    activeSessionSecret = null
    creatingProjectNote = false
    creatingProjectSecret = false
    creatingSessionNote = false
    creatingSessionSecret = false
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
    activeSessionSecret = null
    creatingSessionSecret = false
    editingSessionSecret = false
    sessionSecretValue = ""
    sessionSecrets = []
    activeProjectNote = null
    creatingProjectNote = false
    activeProjectSecret = null
    creatingProjectSecret = false
    editingProjectSecret = false
    projectFiles = []
    projectNotes = []
		projectSecrets = []
    if (isSessionNotesRoute()) {
      initializeSessionNotesRoute()
    } else if (isSessionSecretsRoute()) {
      const secretID = routeSessionSecretID(route)
      if (secretID === "new") {
        activateSessionSecretCreate()
      }
    } else {
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
		activeProjectSecret = null
		creatingProjectSecret = false
		editingProjectSecret = false
		projectSecretValue = ""
    projectFiles = []
    projectFileError = ""
    projectNotes = []
		projectSecrets = []
    events = []
    const secretID = routeProjectSecretID(route)
    if (secretID === "new") {
      activateProjectSecretCreate()
    }
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

  async function loadSessionSearch(reset = false) {
    if (activeWorkspace === null || !isChatCollection() || sessionSearchLoading && !reset || !reset && sessionSearchCursor === null) {
      return
    }
    const workspace = activeWorkspace
    const name = collectionSearchName()
    const cursor = reset ? "" : sessionSearchCursor ?? ""
    const generation = reset ? ++sessionSearchGeneration : sessionSearchGeneration
    if (reset) {
      chatSearch = name
      searchedSessions = []
      sessionSearchCursor = null
    }
    sessionSearchLoading = true
    try {
      const parameters = new URLSearchParams({ limit: "50" })
      if (name.trim() !== "") {
        parameters.set("name", name)
      }
      if (cursor !== "") {
        parameters.set("cursor", cursor)
      }
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions?${parameters}`, { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("sessions could not be searched")
      }
      const loaded = (await response.json()) as SessionSearchResponse
      if (generation !== sessionSearchGeneration || activeWorkspace?.id !== workspace.id || !isChatCollection()) {
        return
      }
      searchedSessions = reset ? loaded.sessions : [...searchedSessions, ...loaded.sessions]
      sessionSearchCursor = loaded.next_cursor ?? null
    } finally {
      if (generation === sessionSearchGeneration) {
        sessionSearchLoading = false
      }
    }
  }

  async function loadProjectSearch(reset = false) {
    if (activeWorkspace === null || !isProjectCollection() || projectSearchLoading && !reset || !reset && projectSearchCursor === null) {
      return
    }
    const workspace = activeWorkspace
    const name = collectionSearchName()
    const cursor = reset ? "" : projectSearchCursor ?? ""
    const generation = reset ? ++projectSearchGeneration : projectSearchGeneration
    if (reset) {
      projectSearch = name
      searchedProjects = []
      projectSearchCursor = null
    }
    projectSearchLoading = true
    try {
      const parameters = new URLSearchParams({ limit: "50" })
      if (name.trim() !== "") {
        parameters.set("name", name)
      }
      if (cursor !== "") {
        parameters.set("cursor", cursor)
      }
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects?${parameters}`, { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("projects could not be searched")
      }
      const loaded = (await response.json()) as ProjectSearchResponse
      if (generation !== projectSearchGeneration || activeWorkspace?.id !== workspace.id || !isProjectCollection()) {
        return
      }
      searchedProjects = reset ? loaded.projects : [...searchedProjects, ...loaded.projects]
      projectSearchCursor = loaded.next_cursor ?? null
    } finally {
      if (generation === projectSearchGeneration) {
        projectSearchLoading = false
      }
    }
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

	function projectSecretsAPIPath(workspace: Workspace, project: Project) {
		return `/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects/${encodeURIComponent(project.id)}/secrets`
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
			const response = await fetch(projectSecretsAPIPath(workspace, project), { credentials: "same-origin" })
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

	async function loadProjectSecret(project: Project, id: string) {
		if (activeWorkspace === null) {
			return null
		}
		const workspace = activeWorkspace
		const response = await fetch(`${projectSecretsAPIPath(workspace, project)}/${encodeURIComponent(id)}`, { credentials: "same-origin" })
		if (response.status === 401) {
			signInRequired()
			return null
		}
		if (response.status === 404) {
			return null
		}
		if (!response.ok) {
			throw new Error("project secret could not be loaded")
		}
		return (await response.json()) as ProjectSecret
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

  async function loadSessionSecrets(session: Session, showLoading = true, projection?: ActivityProjection) {
    if (activeWorkspace === null || !isSessionSecretsRoute()) {
      return false
    }
    const workspace = activeWorkspace
    if (showLoading) {
      sessionSecretStatus = "checking"
    }
    try {
      const response = await fetch(sessionSecretsAPIPath(workspace, session), { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return false
      }
      if (!response.ok) {
        throw new Error("session secrets could not be loaded")
      }
      const loaded = (await response.json()) as SessionSecret[]
      if (projection !== undefined && !isActiveProjection(projection) || activeWorkspace?.id !== workspace.id || activeSession?.id !== session.id || !isSessionSecretsRoute()) {
        return false
      }
      sessionSecrets = [...loaded].sort((left, right) => {
        const difference = new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime()
        return Number.isFinite(difference) && difference !== 0 ? difference : right.id.localeCompare(left.id)
      })
      sessionSecretStatus = "ready"
      return true
    } catch {
      if ((projection === undefined || isActiveProjection(projection)) && activeWorkspace?.id === workspace.id && activeSession?.id === session.id && isSessionSecretsRoute()) {
        sessionSecretStatus = "unavailable"
      }
      return false
    }
  }

  async function loadSessionSecret(session: Session, id: string) {
    if (activeWorkspace === null || !isSessionSecretsRoute()) {
      return null
    }
    const workspace = activeWorkspace
    const response = await fetch(`${sessionSecretsAPIPath(workspace, session)}/${encodeURIComponent(id)}`, { credentials: "same-origin" })
    if (response.status === 401) {
      signInRequired()
      return null
    }
    if (response.status === 404) {
      return null
    }
    if (!response.ok) {
      throw new Error("session secret could not be loaded")
    }
    return (await response.json()) as SessionSecret
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

  const activity = createActivityClient({ onAuthenticationLost: signInRequired, onPollComplete: () => {
    activityPollTimestamp = Date.now()
  } })

  function stopActivityPolling() {
    disposeActiveProjection()
    activity.dispose()
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

  async function refreshWorkspaceGroups(workspace: Workspace, projection?: ActivityProjection) {
    const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/groups`, { credentials: "same-origin" })
    if (response.status === 401) {
      signInRequired()
      return false
    }
    if (!response.ok) {
      throw new Error("groups could not be refreshed")
    }
    const loaded = (await response.json()) as Group[]
    if ((projection !== undefined && !isActiveProjection(projection)) || activeWorkspace?.id !== workspace.id || !isGroupCollection()) {
      return false
    }
    groups = loaded
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
    const principalID = auth.state.claims?.principal.ref.id
    if (principalID === undefined) {
      return
    }
    const principalSelector: ActivitySelector = {
      name: "principal",
      topic: principalID,
      events: ["workspace_grant.*", "group_member.*", "system_grant.*"],
    }
    const systemSelector = systemAccess === "available" ? {
      name: "system",
      topic: "sys",
      events: ["system_grant.*"],
    } satisfies ActivitySelector : undefined
    if (activeWorkspace === null) {
      let unsubscribe: (() => void) | undefined
      const projection: ActivityProjection = { stop: () => unsubscribe?.() }
      activeProjection = projection
      unsubscribe = activity.subscribe([principalSelector, systemSelector].filter((selector): selector is ActivitySelector => selector !== undefined), async ({ names, signal }) => {
        if (signal.aborted || !isActiveProjection(projection)) {
          return
        }
        const principalChanged = names.has(principalSelector.name)
        if (principalChanged || names.has(systemSelector?.name ?? "")) {
          await loadSystemGrants()
        }
        const refreshed = !principalChanged || await loadWorkspaces()
        if (!refreshed || signal.aborted || !isActiveProjection(projection)) {
          throw new Error("activity projection refresh failed")
        }
      })
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
    const groupSelector = isGroupCollection() ? {
      name: "group",
      topic: workspace.id,
      events: ["group.*", "group_member.*"],
    } satisfies ActivitySelector : undefined
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
      events: ["session.*", "session_event.*", "session_file.*", "session_note.*", "session_secret.*"],
    } satisfies ActivitySelector
    const projectID = session?.project?.id ?? activeProject?.id
    const projectSelector = projectID === undefined ? (overview ? {
      name: "project",
      topic: workspace.id,
      events: ["project.*"],
    } satisfies ActivitySelector : undefined) : {
      name: "project",
      topic: `${workspace.id}/${projectID}`,
      events: ["project.*", "project_file.*", "project_note.*", "project_secret.*", "session.*"],
    } satisfies ActivitySelector
    const activitySelectors = [principalSelector, systemSelector, workspaceSelector, groupSelector, agentSelector, sessionSelector, projectSelector].filter((selector): selector is ActivitySelector => selector !== undefined)
    let unsubscribe: (() => void) | undefined
    const projection: ActivityProjection = { stop: () => unsubscribe?.() }
    activeProjection = projection
    unsubscribe = activity.subscribe(activitySelectors, async ({ names, signal }) => {
      if (signal.aborted || !isActiveProjection(projection)) {
        return
      }

      const principalChanged = names.has(principalSelector.name)
      const systemChanged = systemSelector !== undefined && names.has(systemSelector.name)
      const workspaceChanged = names.has(workspaceSelector.name)
      const groupChanged = groupSelector !== undefined && names.has(groupSelector.name)
      const agentChanged = agentSelector !== undefined && names.has(agentSelector.name)
      const sessionChanged = sessionSelector !== undefined && names.has(sessionSelector.name)
      const projectChanged = projectSelector !== undefined && names.has(projectSelector.name)

      if (principalChanged || systemChanged) {
        await loadSystemGrants()
      }
      let refreshed = (await Promise.all([
        ...(principalChanged || workspaceChanged ? [loadWorkspaces(false)] : []),
        ...(sessionSelector !== undefined ? [refreshWorkspaceSessions(workspace, projection)] : []),
        ...(projectSelector !== undefined ? [refreshWorkspaceProjects(workspace, projection)] : []),
        ...(groupChanged ? [refreshWorkspaceGroups(workspace, projection)] : []),
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
        } else if (isSessionSecretsRoute()) {
          if (sessionChanged) {
            refreshed = await loadSessionSecrets(session, false, projection)
            const secretID = routeSessionSecretID(route)
            if (refreshed && isActiveProjection(projection) && secretID !== null && secretID !== "new") {
              const secret = await loadSessionSecret(session, secretID)
              if (!isActiveProjection(projection)) {
                throw new Error("activity projection refresh failed")
              }
              if (secret === null) {
                activeSessionSecret = null
                editingSessionSecret = false
                sessionSecretValue = ""
                navigate(sessionSecretsPath(workspace, session), true)
              } else {
                activeSessionSecret = secret
              }
            } else if (refreshed && isActiveProjection(projection) && activeSessionSecret !== null) {
              const secret = activeSessionSecret
              const loaded = await loadSessionSecret(session, secret.id)
              if (!isActiveProjection(projection)) {
                throw new Error("activity projection refresh failed")
              }
              if (loaded === null && activeSessionSecret?.id === secret.id) {
                activeSessionSecret = null
                editingSessionSecret = false
                sessionSecretValue = ""
              } else if (loaded !== null && activeSessionSecret?.id === secret.id) {
                activeSessionSecret = loaded
              }
            }
          }
        } else {
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
		const secretID = routeProjectSecretID(route)
		if (refreshed && isActiveProjection(projection) && secretID !== null && secretID !== "new") {
			const secret = await loadProjectSecret(activeProject, secretID)
			if (!isActiveProjection(projection)) {
				throw new Error("activity projection refresh failed")
			}
			if (secret === null) {
				activeProjectSecret = null
				editingProjectSecret = false
				projectSecretValue = ""
				navigate(projectSecretsPath(workspace, activeProject), true)
			} else {
				activeProjectSecret = secret
			}
		} else if (refreshed && isActiveProjection(projection) && activeProjectSecret !== null) {
			const secret = activeProjectSecret
			const loaded = await loadProjectSecret(activeProject, secret.id)
			if (!isActiveProjection(projection)) {
				throw new Error("activity projection refresh failed")
			}
			if (loaded === null && activeProjectSecret?.id === secret.id) {
				activeProjectSecret = null
				editingProjectSecret = false
				projectSecretValue = ""
			} else if (loaded !== null && activeProjectSecret?.id === secret.id) {
				activeProjectSecret = loaded
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

	function activateProjectSecretCreate() {
		if (activeWorkspace === null || activeProject === null) {
			return
		}
		activeProjectSecret = null
		creatingProjectSecret = true
		editingProjectSecret = true
		projectSecretDescription = ""
		projectSecretValue = ""
		projectSecretError = ""
	}

	function startProjectSecretCreate() {
		if (activeWorkspace !== null && activeProject !== null) {
			navigate(projectSecretPath(activeWorkspace, activeProject, "new"))
		}
	}

	function startProjectSecretEdit() {
		if (activeProjectSecret === null) {
			return
		}
		projectSecretDescription = activeProjectSecret.description
		projectSecretValue = ""
		projectSecretError = ""
		editingProjectSecret = true
	}

	function cancelProjectSecretEdit() {
		if (savingProjectSecret || activeWorkspace === null || activeProject === null) {
			return
		}
		projectSecretError = ""
		projectSecretValue = ""
		if (creatingProjectSecret) {
			creatingProjectSecret = false
			editingProjectSecret = false
			navigate(projectSecretsPath(activeWorkspace, activeProject))
			return
		}
		editingProjectSecret = false
	}

	async function saveProjectSecret() {
		if (activeWorkspace === null || activeProject === null || projectSecretDescription.trim() === "") {
			projectSecretError = "Description is required."
			return
		}
		if (creatingProjectSecret && projectSecretValue === "") {
			projectSecretError = "Value is required."
			return
		}
		const workspace = activeWorkspace
		const project = activeProject
		const creating = creatingProjectSecret
		const secret = activeProjectSecret
		const input: { description: string; value?: string } = { description: projectSecretDescription }
		if (creating || projectSecretValue !== "") {
			input.value = projectSecretValue
		}
		projectSecretError = ""
		savingProjectSecret = true
		try {
			const secretsPath = projectSecretsAPIPath(workspace, project)
			const path = creating ? secretsPath : `${secretsPath}/${encodeURIComponent(secret?.id ?? "")}`
			const response = await fetch(path, {
				method: creating ? "POST" : "PATCH",
				credentials: "same-origin",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify(input),
			})
			if (response.status === 401) {
				signInRequired()
				return
			}
			if (!response.ok) {
				throw new Error("project secret could not be saved")
			}
			const saved = (await response.json()) as ProjectSecret
			if (activeWorkspace?.id !== workspace.id || activeProject?.id !== project.id) {
				return
			}
			projectSecretValue = ""
			activeProjectSecret = saved
			creatingProjectSecret = false
			editingProjectSecret = false
			projectSecrets = [saved, ...projectSecrets.filter((candidate) => candidate.id !== saved.id)]
			navigate(projectSecretPath(workspace, project, saved))
		} catch {
			projectSecretError = "The secret could not be saved. Try again."
		} finally {
			savingProjectSecret = false
		}
	}

	async function removeProjectSecret() {
		if (activeWorkspace === null || activeProject === null || activeProjectSecret === null || deletingProjectSecret || !window.confirm(`Remove ${activeProjectSecret.description}?`)) {
			return
		}
		const workspace = activeWorkspace
		const project = activeProject
		const secret = activeProjectSecret
		deletingProjectSecret = true
		projectSecretError = ""
		try {
			const response = await fetch(`${projectSecretsAPIPath(workspace, project)}/${encodeURIComponent(secret.id)}`, { method: "DELETE", credentials: "same-origin" })
			if (response.status === 401) {
				signInRequired()
				return
			}
			if (!response.ok) {
				throw new Error("project secret could not be removed")
			}
			if (activeWorkspace?.id === workspace.id && activeProject?.id === project.id && activeProjectSecret?.id === secret.id) {
				activeProjectSecret = null
				editingProjectSecret = false
				projectSecretValue = ""
				projectSecrets = projectSecrets.filter((candidate) => candidate.id !== secret.id)
				navigate(projectSecretsPath(workspace, project))
			}
		} catch {
			projectSecretError = "The secret could not be removed. Try again."
		} finally {
			deletingProjectSecret = false
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

  function activateSessionSecretCreate() {
    if (activeWorkspace === null || activeSession === null) {
      return
    }
    activeSessionSecret = null
    creatingSessionSecret = true
    editingSessionSecret = true
    sessionSecretDescription = ""
    sessionSecretValue = ""
    sessionSecretError = ""
  }

  function startSessionSecretCreate() {
    if (activeWorkspace !== null && activeSession !== null) {
      navigate(sessionSecretPath(activeWorkspace, activeSession, "new"))
    }
  }

  function startSessionSecretEdit() {
    if (activeSessionSecret === null) {
      return
    }
    sessionSecretDescription = activeSessionSecret.description
    sessionSecretValue = ""
    sessionSecretError = ""
    editingSessionSecret = true
  }

  function cancelSessionSecretEdit() {
    if (savingSessionSecret || activeWorkspace === null || activeSession === null) {
      return
    }
    sessionSecretError = ""
    sessionSecretValue = ""
    if (creatingSessionSecret) {
      creatingSessionSecret = false
      editingSessionSecret = false
      navigate(sessionSecretsPath(activeWorkspace, activeSession))
      return
    }
    editingSessionSecret = false
  }

  async function saveSessionSecret() {
    if (activeWorkspace === null || activeSession === null || sessionSecretDescription.trim() === "") {
      sessionSecretError = "Description is required."
      return
    }
    if (creatingSessionSecret && sessionSecretValue === "") {
      sessionSecretError = "Value is required."
      return
    }
    const workspace = activeWorkspace
    const session = activeSession
    const creating = creatingSessionSecret
    const secret = activeSessionSecret
    const input: { description: string; value?: string } = { description: sessionSecretDescription }
    if (creating || sessionSecretValue !== "") {
      input.value = sessionSecretValue
    }
    sessionSecretError = ""
    savingSessionSecret = true
    try {
      const secretsPath = sessionSecretsAPIPath(workspace, session)
      const path = creating ? secretsPath : `${secretsPath}/${encodeURIComponent(secret?.id ?? "")}`
      const response = await fetch(path, {
        method: creating ? "POST" : "PATCH",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("session secret could not be saved")
      }
      const saved = (await response.json()) as SessionSecret
      if (activeWorkspace?.id !== workspace.id || activeSession?.id !== session.id) {
        return
      }
      sessionSecretValue = ""
      activeSessionSecret = saved
      creatingSessionSecret = false
      editingSessionSecret = false
      sessionSecrets = [saved, ...sessionSecrets.filter((candidate) => candidate.id !== saved.id)]
      navigate(sessionSecretPath(workspace, session, saved))
    } catch {
      sessionSecretError = "The secret could not be saved. Try again."
    } finally {
      savingSessionSecret = false
    }
  }

  async function removeSessionSecret() {
    if (activeWorkspace === null || activeSession === null || activeSessionSecret === null || deletingSessionSecret || !window.confirm(`Remove ${activeSessionSecret.description}?`)) {
      return
    }
    const workspace = activeWorkspace
    const session = activeSession
    const secret = activeSessionSecret
    deletingSessionSecret = true
    sessionSecretError = ""
    try {
      const response = await fetch(`${sessionSecretsAPIPath(workspace, session)}/${encodeURIComponent(secret.id)}`, { method: "DELETE", credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("session secret could not be removed")
      }
      if (activeWorkspace?.id === workspace.id && activeSession?.id === session.id && activeSessionSecret?.id === secret.id) {
        activeSessionSecret = null
        editingSessionSecret = false
        sessionSecretValue = ""
        sessionSecrets = sessionSecrets.filter((candidate) => candidate.id !== secret.id)
        navigate(sessionSecretsPath(workspace, session))
      }
    } catch {
      sessionSecretError = "The secret could not be removed. Try again."
    } finally {
      deletingSessionSecret = false
    }
  }

  function sessionSecretsAPIPath(workspace: Workspace, session: Session) {
    return `/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions/${encodeURIComponent(session.id)}/secrets`
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

  async function login() {
    loginError = ""
    submitting = true
    try {
      if (!await signIn(identity, password)) {
        loginError = "The username or password is incorrect."
        return
      }
      password = ""
      await checkSession()
    } catch {
      loginError = "Gatehouse could not be reached. Try again."
    } finally {
      submitting = false
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

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && workspaceStatus === "checking")}
  <main class="auth-shell" aria-busy="true" aria-live="polite">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <div class="loading-mark" aria-hidden="true"></div>
      <p>{auth.state.status === "checking" ? "Checking your session." : "Loading your workspaces."}</p>
    </section>
  </main>
{:else if auth.state.status === "unavailable" || workspaceStatus === "unavailable"}
  <main class="auth-shell">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">Connection unavailable</h1>
      <p class="subtitle is-6">Gatehouse could not load your account.</p>
      <button class="button is-primary" type="button" onclick={() => void checkSession()}>Try again</button>
    </section>
  </main>
{:else if auth.state.status === "anonymous"}
  <main class="auth-shell">
    <section class="login-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-2">Welcome back.</h1>
      <p class="subtitle is-6">Sign in to continue to your workspace.</p>
      <form autocomplete="off" onsubmit={(event) => { event.preventDefault(); void login() }}>
        <div class="field">
          <label class="label" for="identity">Username</label>
          <div class="control">
            <input class="input" id="identity" name="identity" autocomplete="off" required bind:value={identity} />
          </div>
        </div>
        <div class="field">
          <label class="label" for="password">Password</label>
          <div class="control">
            <input class="input" id="password" name="password" type="password" autocomplete="off" required bind:value={password} />
          </div>
        </div>
        {#if loginError !== ""}
          <p class="help is-danger" aria-live="polite">{loginError}</p>
        {/if}
        <div class="field login-action">
          <div class="control">
            <button class="button is-primary is-fullwidth" type="submit" disabled={submitting}>
              {submitting ? "Signing in..." : "Sign in"}
            </button>
          </div>
        </div>
      </form>
    </section>
  </main>
{:else if isSystemRoute()}
  <div class="app-shell">
    {#if mobileMenuOpen}<button class="mobile-menu-backdrop" type="button" aria-label="Close navigation menu" onclick={() => mobileMenuOpen = false}></button>{/if}
    <aside class:mobile-menu-open={mobileMenuOpen} class="sidebar">
      <a class="brand" href="/app/">Gatehouse</a>
      <nav class="sidebar-nav" aria-label="System navigation">
        <section class="sidebar-section">
          <h2>System</h2>
          <ul>
            <li><a class:active={route.kind === "system"} href="/app/system" onclick={(event) => { event.preventDefault(); navigate("/app/system") }}>Overview</a></li>
            {#if systemAccess === "available"}<li><a class:active={route.kind === "system-principals"} href="/app/system/principals" onclick={(event) => { event.preventDefault(); navigate("/app/system/principals") }}>Principals</a></li>{/if}
            {#if systemAccess === "available"}<li><a class:active={route.kind === "system-grants"} href="/app/system/grants" onclick={(event) => { event.preventDefault(); navigate("/app/system/grants") }}>System grants</a></li>{/if}
          </ul>
        </section>
      </nav>
      <div class="sidebar-footer">
        <span>{auth.state.claims?.principal.name ?? "User"}</span>
        <button class="button is-small is-danger is-light" type="button" onclick={() => void logout()}>Log out</button>
      </div>
    </aside>
    <main class="workspace-main">
      <header class="workspace-header system-header">
        <button class="mobile-menu-trigger" type="button" aria-label="Open navigation menu" aria-expanded={mobileMenuOpen} onclick={() => mobileMenuOpen = true}><Menu size={20} strokeWidth={2} aria-hidden="true" /></button>
        <h1 class="workspace-breadcrumb">
          {#if route.kind === "system-grants" || route.kind === "system-principals"}
            <a class="workspace-breadcrumb-segment" href="/app/system" onclick={(event) => { event.preventDefault(); navigate("/app/system") }}><ShieldCheck size={18} strokeWidth={2} aria-hidden="true" /><span>System</span></a>
            <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
            <span>{route.kind === "system-principals" ? "Principals" : "System grants"}</span>
          {:else}
            <span class="workspace-breadcrumb-segment"><ShieldCheck size={18} strokeWidth={2} aria-hidden="true" /><span>System</span></span>
          {/if}
        </h1>
      </header>
      {#if systemAccess === "checking"}
        <section class="system-page"><p class="dashboard-empty">Loading system access...</p></section>
      {:else if systemAccess !== "available"}
        <section class="system-page system-access-denied"><p class="eyebrow">System</p><h2 class="title is-3">System access required</h2><p>You do not currently have an enabled system manager grant.</p></section>
      {:else if route.kind === "system"}
        <section class="system-page">
          <p class="eyebrow">System</p>
          <h2 class="title is-3">System administration</h2>
          <p class="subtitle is-6">Manage global Gatehouse state.</p>
          <a class="system-section-link" href="/app/system/principals" onclick={(event) => { event.preventDefault(); navigate("/app/system/principals") }}>
            <span><strong>Principals</strong><small>View and enable or disable principals and their identities.</small></span>
          </a>
          <a class="system-section-link" href="/app/system/grants" onclick={(event) => { event.preventDefault(); navigate("/app/system/grants") }}>
            <span><strong>System grants</strong><small>Grant or revoke system-manager access.</small></span>
          </a>
        </section>
      {:else if route.kind === "system-principals"}
        <section class="system-page">
          <div class="system-page-heading"><div><p class="eyebrow">System</p><h2 class="title is-3">Principals</h2><p class="subtitle is-6">Identity associations are shown without credential verifiers.</p></div></div>
          {#if systemPrincipalError !== ""}<p class="help is-danger" aria-live="polite">{systemPrincipalError}</p>{/if}
          <div class="system-principal-list">
            {#each systemPrincipals as principal (principal.id)}
              <article class:system-principal-disabled={!principal.enabled} class="system-principal-row">
                <div><strong>{principal.name ?? principal.alias ?? principal.id}</strong><small>{principal.id}{principal.alias === undefined ? "" : ` / ${principal.alias}`} / revision {principal.revision}</small>{#if principal.identities.length > 0}<div class="system-principal-identities">{#each principal.identities as identity (identity.id)}<span class:has-text-grey={!identity.enabled}>{identity.key} / {identity.id} / revision {identity.revision}{identity.enabled ? "" : " / Disabled"}</span>{/each}</div>{:else}<small>No identities</small>{/if}</div>
                <div class="system-principal-actions"><span class:has-text-success={principal.enabled} class:has-text-grey={!principal.enabled}>{principal.enabled ? "Enabled" : "Disabled"}</span><button class="button is-small" type="button" disabled={updatingSystemPrincipalIDs.has(principal.id)} onclick={() => void setSystemPrincipalEnabled(principal, !principal.enabled)}>{updatingSystemPrincipalIDs.has(principal.id) ? "Saving..." : principal.enabled ? "Disable" : "Enable"}</button></div>
              </article>
            {:else}<p class="dashboard-empty">No principals are configured.</p>{/each}
          </div>
        </section>
      {:else}
        <section class="system-page">
          <div class="system-page-heading"><div><p class="eyebrow">System</p><h2 class="title is-3">System grants</h2><p class="subtitle is-6">System managers can modify global Gatehouse state.</p></div></div>
          <form class="system-grant-form" onsubmit={(event) => { event.preventDefault(); void createSystemGrant() }}>
            <label class="field"><span class="label">Principal ID</span><input class="input" autocomplete="off" placeholder="prn_..." bind:value={systemGrantPrincipal} /></label>
            <button class="button is-primary" type="submit" disabled={creatingSystemGrant}>{creatingSystemGrant ? "Granting..." : "Add manager"}</button>
          </form>
          {#if systemGrantError !== ""}<p class="help is-danger" aria-live="polite">{systemGrantError}</p>{/if}
          <div class="system-grant-list">
            {#each systemGrants as grant (grant.ref.id)}
              <article class:system-grant-disabled={!grant.enabled} class="system-grant-row">
                <div><strong>{grant.principal.id}</strong><small>{grant.ref.id} / revision {grant.revision}</small></div>
                <div class="system-grant-actions"><span class:has-text-success={grant.enabled} class:has-text-grey={!grant.enabled}>{grant.enabled ? "Enabled" : "Disabled"}</span><button class="button is-small" type="button" disabled={updatingSystemGrantIDs.has(grant.ref.id)} onclick={() => void setSystemGrantEnabled(grant, !grant.enabled)}>{updatingSystemGrantIDs.has(grant.ref.id) ? "Saving..." : grant.enabled ? "Disable" : "Enable"}</button></div>
              </article>
            {:else}<p class="dashboard-empty">No system grants are configured.</p>{/each}
          </div>
        </section>
      {/if}
    </main>
  </div>
{:else if workspaceStatus === "empty"}
  <main class="auth-shell">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">No workspace access</h1>
      <p class="subtitle is-6">Ask an administrator to add {auth.state.claims?.principal.name ?? "User"} to a workspace group.</p>
      {#if systemAccess === "available"}<button class="button is-primary is-light is-fullwidth" type="button" onclick={() => navigate("/app/system")}>System</button>{/if}
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
            const workspace = workspaces.find((candidate) => candidate.id === target.value)
            if (workspace !== undefined) {
              navigate(workspacePath(workspace))
            }
          }}>
            {#each workspaces as workspace}
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

      {#if systemAccess === "available"}
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
				{#if activeSession === null && activeProjectNote === null && !creatingProjectNote && activeProjectSecret === null && !creatingProjectSecret && !isProjectNotesRoute() && !isProjectSecretsRoute()}
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
                {#if activeSessionSecret !== null || creatingSessionSecret}
                  <a href={activeWorkspace !== null ? sessionSecretsPath(activeWorkspace, activeSession) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionSecretsPath(activeWorkspace, activeSession)) } }}>Secrets</a>
                {:else}
                  <span>Secrets</span>
                {/if}
              {:else}
                <span class="workspace-breadcrumb-segment"><MessageSquare size={16} strokeWidth={2} aria-hidden="true" />{activeSession.name ?? "New Chat"}</span>
              {/if}
			{:else if activeProject !== null && (isProjectSecretsRoute() || activeProjectSecret !== null || creatingProjectSecret)}
				<span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
				{#if activeProjectSecret !== null || creatingProjectSecret}
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
            {#if activeSession !== null && (activeSessionSecret !== null || creatingSessionSecret)}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              <span class="workspace-breadcrumb-segment"><Lock size={16} strokeWidth={2} aria-hidden="true" />{activeSessionSecret?.description ?? "New Secret"}</span>
            {/if}
			{#if activeSession === null && activeProject !== null && (activeProjectSecret !== null || creatingProjectSecret)}
				<span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
                <span class="workspace-breadcrumb-segment"><Lock size={16} strokeWidth={2} aria-hidden="true" />{activeProjectSecret?.description ?? "New Secret"}</span>
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
		{#if activeSession === null && activeProject !== null && activeProjectNote === null && !creatingProjectNote && activeProjectSecret === null && !creatingProjectSecret && !isProjectSecretsRoute()}
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
        <section class="dashboard-grid">
          <section class="dashboard-widget dashboard-widget-wide">
            <div class="dashboard-widget-heading"><h2>Latest Chats</h2><button class="button is-primary is-small" type="button" onclick={() => void createSession()}>New chat</button></div>
            {#each latestSessions as session}
              <a class="dashboard-row" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses/${encodeURIComponent(session.id)}`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionPath(activeWorkspace, session)) } }}><span class="dashboard-row-content"><span>{session.name ?? "New Chat"}</span><span class="dashboard-row-meta"><time datetime={session.created_at}>{createdAtLabel(session.created_at)}</time>{#if session.project !== undefined}<span aria-hidden="true">/</span><span>{session.project.name ?? "New Project"}</span>{/if}</span></span></a>
            {:else}<p class="dashboard-empty">No chats yet.</p>{/each}
            {#if latestSessions.length > 0}<a class="dashboard-view-all" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionsPath(activeWorkspace)) } }}>View all chats</a>{/if}
          </section>
          <section class="dashboard-widget dashboard-widget-wide">
            <div class="dashboard-widget-heading"><h2>Latest Projects</h2><button class="button is-primary is-small" type="button" disabled={creatingProject} onclick={() => void createProject()}>New project</button></div>
            {#each latestProjects as project}
              <a class="dashboard-row" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/prj/${encodeURIComponent(project.id)}`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(projectPath(activeWorkspace, project)) } }}><span class="dashboard-row-content"><span>{project.name ?? "New Project"}</span><time datetime={project.created_at}>{createdAtLabel(project.created_at)}</time></span></a>
            {:else}<p class="dashboard-empty">No projects yet.</p>{/each}
            {#if latestProjects.length > 0}<a class="dashboard-view-all" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/prj`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(projectsPath(activeWorkspace)) } }}>View all projects</a>{/if}
          </section>
          {#if messageError !== ""}<p class="help is-danger dashboard-error" aria-live="polite">{messageError}</p>{/if}
        </section>
      {:else if isChatCollection()}
        <section class="collection-page">
          <div class="collection-heading"><h2>Chats</h2><button class="button is-primary is-small" type="button" onclick={() => void createSession()}>New chat</button></div>
          <form class="collection-search" onsubmit={(event) => { event.preventDefault(); void submitSessionSearch() }}>
            <label><span>Search chats</span><input class="input" type="search" autocomplete="off" placeholder="Search chats" bind:value={chatSearch} /></label>
            <button class="button" type="submit" aria-label="Search chats" title="Search chats"><Search size={20} strokeWidth={2} aria-hidden="true" /></button>
          </form>
          <div class="collection-list">
            {#each searchedSessions as session}
              <a class="dashboard-row" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses/${encodeURIComponent(session.id)}`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(sessionPath(activeWorkspace, session)) } }}><span class="dashboard-row-content"><span>{session.name ?? "New Chat"}</span><span class="dashboard-row-meta"><time datetime={session.created_at}>{createdAtLabel(session.created_at)}</time>{#if session.project !== undefined}<span aria-hidden="true">/</span><span>{session.project.name ?? "New Project"}</span>{/if}</span></span></a>
            {:else}<p class="dashboard-empty">{sessionSearchLoading ? "Searching chats..." : "No chats match your search."}</p>{/each}
          </div>
          {#if sessionSearchCursor !== null}<button class="button is-small" type="button" disabled={sessionSearchLoading} onclick={() => void loadSessionSearch()}>{sessionSearchLoading ? "Loading..." : "Show more"}</button>{/if}
        </section>
      {:else if isProjectCollection()}
        <section class="collection-page">
          <div class="collection-heading"><h2>Projects</h2><button class="button is-primary is-small" type="button" disabled={creatingProject} onclick={() => void createProject()}>New project</button></div>
          <form class="collection-search" onsubmit={(event) => { event.preventDefault(); void submitProjectSearch() }}>
            <label><span>Search projects</span><input class="input" type="search" autocomplete="off" placeholder="Search projects" bind:value={projectSearch} /></label>
            <button class="button" type="submit" aria-label="Search projects" title="Search projects"><Search size={20} strokeWidth={2} aria-hidden="true" /></button>
          </form>
          <div class="collection-list">
            {#each searchedProjects as project}
              <a class="dashboard-row" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/prj/${encodeURIComponent(project.id)}`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { navigate(projectPath(activeWorkspace, project)) } }}><span class="dashboard-row-content"><span>{project.name ?? "New Project"}</span><time datetime={project.created_at}>{createdAtLabel(project.created_at)}</time></span></a>
            {:else}<p class="dashboard-empty">{projectSearchLoading ? "Searching projects..." : "No projects match your search."}</p>{/each}
          </div>
          {#if projectSearchCursor !== null}<button class="button is-small" type="button" disabled={projectSearchLoading} onclick={() => void loadProjectSearch()}>{projectSearchLoading ? "Loading..." : "Show more"}</button>{/if}
        </section>
      {:else if isGroupCollection()}
        <section class="collection-page">
          <div class="collection-heading"><h2>Groups</h2></div>
          <div class="collection-search"><label><span>Search groups</span><input class="input" type="search" autocomplete="off" placeholder="Search groups" bind:value={groupSearch} /></label></div>
          <div class="collection-list">
            {#each ordered(groups.filter((group) => matchesSearch(group, groupSearch))) as group}
              <div class="dashboard-row"><span>{group.name ?? "New Group"}</span><small>{group.id}</small></div>
            {:else}<p class="dashboard-empty">No groups match your search.</p>{/each}
          </div>
        </section>
		{:else if activeSession === null && activeProject !== null && isProjectSecretsRoute()}
			<section class="project-note-page">
				{#if editingProjectSecret}
					<form class="project-note-editor" onsubmit={(event) => { event.preventDefault(); void saveProjectSecret() }}>
						<div class="project-note-page-heading"><div><p class="eyebrow">Project Secret</p><h2>{creatingProjectSecret ? "New Secret" : "Edit Secret"}</h2></div></div>
						<div class="field"><label class="label" for="project-secret-description">Description</label><div class="control"><textarea class="textarea" id="project-secret-description" autocomplete="off" rows="3" maxlength="4096" required bind:value={projectSecretDescription}></textarea></div></div>
						<div class="field"><label class="label" for="project-secret-value">{creatingProjectSecret ? "Value" : "New value (optional)"}</label><div class="control"><textarea class="textarea" id="project-secret-value" autocomplete="new-password" rows="5" maxlength="1048576" required={creatingProjectSecret} bind:value={projectSecretValue}></textarea></div>{#if !creatingProjectSecret}<p class="help">Leave blank to keep the current value.</p>{/if}</div>
						{#if projectSecretError !== ""}<p class="help is-danger" aria-live="polite">{projectSecretError}</p>{/if}
						<div class="project-note-actions"><button class="button" type="button" disabled={savingProjectSecret} onclick={cancelProjectSecretEdit}>Cancel</button><button class="button is-primary" type="submit" disabled={savingProjectSecret}>{savingProjectSecret ? "Saving..." : "Save secret"}</button></div>
					</form>
				{:else if activeProjectSecret !== null}
					<article class="project-note-view">
						<header class="project-note-page-heading"><div><p class="eyebrow">Project Secret</p><h2>{activeProjectSecret.description}</h2><small>By {activeProjectSecret.author.name ?? activeProjectSecret.author.id} on {createdAtLabel(activeProjectSecret.created_at)}{#if activeProjectSecret.updated_at !== activeProjectSecret.created_at} / Updated {createdAtLabel(activeProjectSecret.updated_at)}{/if}</small></div><div class="project-note-actions"><button class="button is-small" type="button" onclick={startProjectSecretEdit}>Edit</button><button class="button is-small is-danger is-light" type="button" disabled={deletingProjectSecret} onclick={() => void removeProjectSecret()}>{deletingProjectSecret ? "Removing..." : "Remove"}</button></div></header>
						{#if projectSecretError !== ""}<p class="help is-danger" aria-live="polite">{projectSecretError}</p>{/if}
					</article>
				{:else}
					<div class="collection-heading"><h2>Project Secrets</h2><button class="button is-primary is-small" type="button" onclick={() => startProjectSecretCreate()}>New secret</button></div>
					<div class="collection-list">
						{#if projectSecretStatus === "checking"}
							<p class="dashboard-empty">Loading secrets...</p>
						{:else if projectSecretStatus === "unavailable"}
							<p class="dashboard-empty">Secrets could not be loaded.</p>
						{:else}
							{#each projectSecrets as secret (secret.id)}
								<a class="dashboard-row project-note-row" href={activeWorkspace !== null && activeProject !== null ? projectSecretPath(activeWorkspace, activeProject, secret) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null && activeProject !== null) { navigate(projectSecretPath(activeWorkspace, activeProject, secret)) } }}><span class="dashboard-row-content"><span class="project-note-title">{secret.description}</span><span class="dashboard-row-meta"><span>{secret.author.name ?? secret.author.id}</span><time datetime={secret.updated_at}>Updated {createdAtLabel(secret.updated_at)}</time></span></span></a>
							{:else}<p class="dashboard-empty">No secrets yet.</p>{/each}
						{/if}
					</div>
				{/if}
			</section>
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
        <section class="project-note-page">
          {#if editingSessionSecret}
            <form class="project-note-editor" onsubmit={(event) => { event.preventDefault(); void saveSessionSecret() }}>
              <div class="project-note-page-heading"><div><p class="eyebrow">Session Secret</p><h2>{creatingSessionSecret ? "New Secret" : "Edit Secret"}</h2></div></div>
              <div class="field"><label class="label" for="session-secret-description">Description</label><div class="control"><textarea class="textarea" id="session-secret-description" autocomplete="off" rows="3" maxlength="4096" required bind:value={sessionSecretDescription}></textarea></div></div>
              <div class="field"><label class="label" for="session-secret-value">{creatingSessionSecret ? "Value" : "New value (optional)"}</label><div class="control"><textarea class="textarea" id="session-secret-value" autocomplete="new-password" rows="5" maxlength="1048576" required={creatingSessionSecret} bind:value={sessionSecretValue}></textarea></div>{#if !creatingSessionSecret}<p class="help">Leave blank to keep the current value.</p>{/if}</div>
              {#if sessionSecretError !== ""}<p class="help is-danger" aria-live="polite">{sessionSecretError}</p>{/if}
              <div class="project-note-actions"><button class="button" type="button" disabled={savingSessionSecret} onclick={cancelSessionSecretEdit}>Cancel</button><button class="button is-primary" type="submit" disabled={savingSessionSecret}>{savingSessionSecret ? "Saving..." : "Save secret"}</button></div>
            </form>
          {:else if activeSessionSecret !== null}
            <article class="project-note-view">
              <header class="project-note-page-heading"><div><p class="eyebrow">Session Secret</p><h2>{activeSessionSecret.description}</h2><small>By {activeSessionSecret.author.name ?? activeSessionSecret.author.id} on {createdAtLabel(activeSessionSecret.created_at)}{#if activeSessionSecret.updated_at !== activeSessionSecret.created_at} / Updated {createdAtLabel(activeSessionSecret.updated_at)}{/if}</small></div><div class="project-note-actions"><button class="button is-small" type="button" onclick={startSessionSecretEdit}>Edit</button><button class="button is-small is-danger is-light" type="button" disabled={deletingSessionSecret} onclick={() => void removeSessionSecret()}>{deletingSessionSecret ? "Removing..." : "Remove"}</button></div></header>
              {#if sessionSecretError !== ""}<p class="help is-danger" aria-live="polite">{sessionSecretError}</p>{/if}
            </article>
          {:else}
            <div class="collection-heading"><h2>Session Secrets</h2><button class="button is-primary is-small" type="button" onclick={() => startSessionSecretCreate()}>New secret</button></div>
            <div class="collection-list">
              {#if sessionSecretStatus === "checking"}
                <p class="dashboard-empty">Loading secrets...</p>
              {:else if sessionSecretStatus === "unavailable"}
                <p class="dashboard-empty">Secrets could not be loaded.</p>
              {:else}
                {#each sessionSecrets as secret (secret.id)}
                  <a class="dashboard-row project-note-row" href={activeWorkspace !== null && activeSession !== null ? sessionSecretPath(activeWorkspace, activeSession, secret) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null && activeSession !== null) { navigate(sessionSecretPath(activeWorkspace, activeSession, secret)) } }}><span class="dashboard-row-content"><span class="project-note-title">{secret.description}</span><span class="dashboard-row-meta"><span>{secret.author.name ?? secret.author.id}</span><time datetime={secret.updated_at}>Updated {createdAtLabel(secret.updated_at)}</time></span></span></a>
                {:else}<p class="dashboard-empty">No secrets yet.</p>{/each}
              {/if}
            </div>
          {/if}
        </section>
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
				<div class="dashboard-widget-heading"><h2>Project Secrets</h2><button class="button is-primary is-small" type="button" onclick={() => startProjectSecretCreate()}>New secret</button></div>
				{#if projectSecretStatus === "checking"}
					<p class="dashboard-empty">Loading secrets...</p>
				{:else if projectSecretStatus === "unavailable"}
					<p class="dashboard-empty">Secrets could not be loaded.</p>
				{:else}
					{#each projectSecrets as secret (secret.id)}
						<a class="dashboard-row project-note-row" href={activeWorkspace !== null && activeProject !== null ? projectSecretPath(activeWorkspace, activeProject, secret) : "#"} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null && activeProject !== null) { navigate(projectSecretPath(activeWorkspace, activeProject, secret)) } }}><span class="dashboard-row-content"><span class="project-note-title">{secret.description}</span><span class="dashboard-row-meta"><span>{secret.author.name ?? secret.author.id}</span><time datetime={secret.updated_at}>Updated {createdAtLabel(secret.updated_at)}</time></span></span></a>
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
