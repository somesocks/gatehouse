<script lang="ts">
  import { onMount, tick } from "svelte"
  import { Bot, CircleCheck, CircleX, Copy, Menu, Paperclip, Search, Send, ShieldCheck, ShieldQuestionMark, ShieldX, X } from "@lucide/svelte"
  import { renderMarkdown } from "./markdown"
  import type { ActivityTopicCheckpoint, ActivityTopicCheckpoints } from "./model"

  type Claims = {
    principal: {
      ref: { id: string }
      name?: string
    }
    identity: string
  }

  type Workspace = {
    id: string
    name?: string
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

  type ProjectNote = {
    id: string
    title: string
    description: string
    body?: string
    author: { id: string; name?: string }
    created_at: string
  }

  type SessionNote = ProjectNote

  type SessionSearchResponse = {
    sessions: Session[]
    next_cursor?: string
  }

  type ProjectSearchResponse = {
    projects: Project[]
    next_cursor?: string
  }

  type ActivityCursor = NonNullable<ActivityTopicCheckpoint["cursor"]>

  type WorkspaceAgent = {
    id: string
    label?: string
  }

  type SessionEvent = {
    created_at: string
    kind: string
    payload: { text?: string; name?: string; reason?: string; code?: string; description?: string; output?: string; attachments?: MessageFile[] }
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

  type AuthenticationStatus = "checking" | "anonymous" | "authenticated" | "unavailable"
  type WorkspaceStatus = "checking" | "ready" | "empty" | "unavailable"
  type WorkspaceContentStatus = "checking" | "ready" | "unavailable"

  let status = $state<AuthenticationStatus>("checking")
  let workspaceStatus = $state<WorkspaceStatus>("checking")
  let workspaceContentStatus = $state<WorkspaceContentStatus>("checking")
  let claims = $state<Claims | null>(null)
  let currentPath = $state("/app/")
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
  let projectNoteError = $state("")
  let activeSessionNote = $state<SessionNote | null>(null)
  let creatingSessionNote = $state(false)
  let editingSessionNote = $state(false)
  let savingSessionNote = $state(false)
  let deletingSessionNote = $state(false)
  let sessionNoteTitle = $state("")
  let sessionNoteDescription = $state("")
  let sessionNoteBody = $state("")
  let sessionNoteError = $state("")
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
  let activityPollTimer: ReturnType<typeof setTimeout> | undefined
  let activityPollGeneration = 0
  let activeSessionCursor: ActivityCursor | null = null
  let activeProjectCursor: ActivityCursor | null = null
  let workspaceSessionsCursor: ActivityCursor | null = null
  let workspaceProjectsCursor: ActivityCursor | null = null
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
  let sessionNotes = $state<SessionNote[]>([])
  let sessionNoteStatus = $state<WorkspaceContentStatus>("checking")
  let sessionSearchCursor = $state<string | null>(null)
  let projectSearchCursor = $state<string | null>(null)
  let sessionSearchLoading = $state(false)
  let projectSearchLoading = $state(false)
  let sessionSearchGeneration = 0
  let projectSearchGeneration = 0

  onMount(() => {
    currentPath = window.location.pathname
    const handlePopState = () => {
      currentPath = window.location.pathname
      if (status === "authenticated" && activeWorkspace !== null && workspaceIDFromPath(currentPath) === activeWorkspace.id) {
        void selectWorkspace(activeWorkspace, true)
      } else if (status === "authenticated") {
        void loadWorkspaces()
      } else {
        void checkSession()
      }
    }
    const closeProjectActionMenu = (event: MouseEvent) => {
      if (projectActionMenuElement?.open && event.target instanceof Node && !projectActionMenuElement.contains(event.target)) {
        projectActionMenuElement.open = false
      }
    }
    window.addEventListener("popstate", handlePopState)
    document.addEventListener("click", closeProjectActionMenu)
    void checkSession()
    return () => {
      window.removeEventListener("popstate", handlePopState)
      document.removeEventListener("click", closeProjectActionMenu)
      stopActivityPolling()
    }
  })

  function isLoginPath() {
    return currentPath === "/app/login" || currentPath === "/app/login/"
  }

  function nextPath() {
    const next = new URLSearchParams(window.location.search).get("next")
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

  function workspaceIDFromPath(path: string) {
    const match = /^\/app\/wsp\/([^/]+)(?:\/|$)/.exec(path)
    if (match === null) {
      return null
    }
    try {
      return decodeURIComponent(match[1])
    } catch {
      return null
    }
  }

  function sessionIDFromPath(path: string) {
    const match = /^\/app\/wsp\/[^/]+\/ses\/([^/]+)(?:\/|$)/.exec(path)
    if (match === null) {
      return null
    }
    try {
      return decodeURIComponent(match[1])
    } catch {
      return null
    }
  }

  function projectIDFromPath(path: string) {
    const match = /^\/app\/wsp\/[^/]+\/prj\/([^/]+)(?:\/|$)/.exec(path)
    if (match === null) {
      return null
    }
    try {
      return decodeURIComponent(match[1])
    } catch {
      return null
    }
  }

  function projectNoteIDFromPath(path: string) {
    const match = /^\/app\/wsp\/[^/]+\/prj\/[^/]+\/pnt\/([^/]+)\/?$/.exec(path)
    if (match === null) {
      return null
    }
    try {
      return decodeURIComponent(match[1])
    } catch {
      return null
    }
  }

  function sessionNoteIDFromPath(path: string) {
    const match = /^\/app\/wsp\/[^/]+\/ses\/[^/]+\/notes\/([^/]+)\/?$/.exec(path)
    if (match === null) {
      return null
    }
    try {
      return decodeURIComponent(match[1])
    } catch {
      return null
    }
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

  function projectsPath(workspace: Workspace) {
    return `${workspacePath(workspace)}/prj`
  }

  function projectNotePath(workspace: Workspace, project: Project, note: ProjectNote | string) {
    const id = typeof note === "string" ? note : note.id
    return `${projectsPath(workspace)}/${encodeURIComponent(project.id)}/pnt/${encodeURIComponent(id)}`
  }

  function groupsPath(workspace: Workspace) {
    return `${workspacePath(workspace)}/grp`
  }

  function collectionSearchName() {
    return new URLSearchParams(window.location.search).get("name") ?? ""
  }

  function collectionSearchPath(path: string, name: string) {
    const query = name.trim()
    return query === "" ? path : `${path}?${new URLSearchParams({ name: query })}`
  }

  function isCollectionPath(collection: "ses" | "prj" | "grp") {
    return new RegExp(`^/app/wsp/[^/]+/${collection}/?$`).test(currentPath)
  }

  function isChatCollection() {
    return isCollectionPath("ses")
  }

  function isSessionNotesRoute() {
    return /^\/app\/wsp\/[^/]+\/ses\/[^/]+\/notes(?:\/[^/]+)?\/?$/.test(currentPath)
  }

  function isProjectCollection() {
    return isCollectionPath("prj")
  }

  function isGroupCollection() {
    return isCollectionPath("grp")
  }

  async function selectWorkspaceRoute(path: string) {
    if (activeWorkspace === null) {
      return
    }
    navigate(path)
    await selectWorkspace(activeWorkspace, true)
  }

  async function submitSessionSearch() {
    if (activeWorkspace === null) {
      return
    }
    navigate(collectionSearchPath(sessionsPath(activeWorkspace), chatSearch))
    await loadSessionSearch(true)
  }

  async function submitProjectSearch() {
    if (activeWorkspace === null) {
      return
    }
    navigate(collectionSearchPath(projectsPath(activeWorkspace), projectSearch))
    await loadProjectSearch(true)
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

  function navigate(path: string, replace = true) {
    window.history[replace ? "replaceState" : "pushState"](null, "", path)
    currentPath = new URL(path, window.location.origin).pathname
  }

  function redirectToLogin() {
    const requested = window.location.pathname + window.location.search + window.location.hash
    navigate(`/app/login?next=${encodeURIComponent(requested)}`)
  }

  function signInRequired() {
    stopActivityPolling()
    claims = null
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
    activeProject = null
    activeProjectNote = null
    creatingProjectNote = false
    projectFiles = []
    projectFileError = ""
    projectNotes = []
    events = []
    showJumpToLatest = false
    status = "anonymous"
    workspaceStatus = "checking"
    if (!isLoginPath()) {
      redirectToLogin()
    }
  }

  async function checkSession(preferFirstWorkspace = false) {
    status = "checking"
    try {
      const response = await fetch("/api/v1/auth/me", { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error(`authentication check returned ${response.status}`)
      }
      claims = (await response.json()) as Claims
      status = "authenticated"
      await loadWorkspaces(preferFirstWorkspace)
    } catch {
      claims = null
      status = "unavailable"
    }
  }

  async function loadWorkspaces(preferFirstWorkspace = false) {
    workspaceStatus = "checking"
    try {
      const response = await fetch("/api/v1/workspaces", { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return
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
        navigate("/app/no-access")
        return
      }
      workspaceStatus = "ready"
      const requestedPath = isLoginPath() ? nextPath() : currentPath
      const requestedID = preferFirstWorkspace ? null : workspaceIDFromPath(requestedPath)
      const workspace = workspaces.find((candidate) => candidate.id === requestedID) ?? workspaces[0]
      if (requestedID !== null && workspace.id === requestedID && isLoginPath()) {
        navigate(requestedPath)
      }
      await selectWorkspace(workspace, true)
    } catch {
      workspaceStatus = "unavailable"
    }
  }

  async function selectWorkspace(workspace: Workspace, replace = false) {
    mobileMenuOpen = false
    stopActivityPolling(true)
    activeWorkspace = workspace
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
    activeProject = null
    activeProjectNote = null
    creatingProjectNote = false
    projectFiles = []
    projectFileError = ""
    projectNotes = []
    events = []
    showJumpToLatest = false
    workspaceContentStatus = "checking"
    if (workspaceIDFromPath(currentPath) !== workspace.id) {
      navigate(workspacePath(workspace), replace)
    }
    try {
      const [groupsResponse, projectsResponse, sessionsResponse, agentsResponse] = await Promise.all([
        fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/groups`, { credentials: "same-origin" }),
        fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects?limit=5`, { credentials: "same-origin" }),
        fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions?limit=5`, { credentials: "same-origin" }),
        fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/agents`, { credentials: "same-origin" }),
      ])
      if (groupsResponse.status === 401 || projectsResponse.status === 401 || sessionsResponse.status === 401 || agentsResponse.status === 401) {
        signInRequired()
        return
      }
      if (!groupsResponse.ok || !projectsResponse.ok || !sessionsResponse.ok || !agentsResponse.ok) {
        throw new Error("workspace data could not be loaded")
      }
      groups = (await groupsResponse.json()) as Group[]
      latestProjects = ((await projectsResponse.json()) as ProjectSearchResponse).projects
      latestSessions = ((await sessionsResponse.json()) as SessionSearchResponse).sessions
      agents = (await agentsResponse.json()) as WorkspaceAgent[]
      workspaceContentStatus = "ready"
      const sessionID = sessionIDFromPath(currentPath)
      const session = sessionID === null ? undefined : latestSessions.find((candidate) => candidate.id === sessionID) ?? await loadSession(workspace, sessionID)
      if (session !== undefined && session !== null) {
        await selectSession(session, true)
      } else {
        const projectID = projectIDFromPath(currentPath)
        activeProject = projectID === null ? null : latestProjects.find((candidate) => candidate.id === projectID) ?? await loadProject(workspace, projectID)
        if (activeProject !== null) {
          await Promise.all([loadProjectSessions(activeProject), loadProjectFiles(activeProject), loadProjectNotes(activeProject)])
          const noteID = projectNoteIDFromPath(currentPath)
          if (noteID === "new") {
            startProjectNoteCreate()
          } else if (noteID !== null) {
            activeProjectNote = await loadProjectNote(activeProject, noteID)
            if (activeProjectNote === null) {
              navigate(`${projectsPath(workspace)}/${encodeURIComponent(activeProject.id)}`, true)
            }
          }
        } else if (isChatCollection()) {
          await loadSessionSearch(true)
        } else if (isProjectCollection()) {
          await loadProjectSearch(true)
        }
        startActivityPolling()
      }
    } catch {
      workspaceContentStatus = "unavailable"
    }
  }

  async function selectSession(session: Session, replace = false) {
    if (activeWorkspace === null) {
      return
    }
    mobileMenuOpen = false
    stopActivityPolling(false)
    activeSessionCursor = null
    activeSession = session
    activeSessionNote = null
    creatingSessionNote = false
    editingSessionNote = false
    sessionNotes = []
    activeProject = session.project ?? null
    activeProjectNote = null
    creatingProjectNote = false
    projectFiles = []
    projectFileError = ""
    events = []
    showJumpToLatest = false
    eventStatus = "checking"
    messageError = ""
    if (sessionIDFromPath(currentPath) !== session.id) {
      navigate(sessionPath(activeWorkspace, session), replace)
    }
    if (isSessionNotesRoute()) {
      await loadSessionNotes(session)
      const noteID = sessionNoteIDFromPath(currentPath)
      if (noteID === "new") {
        startSessionNoteCreate(false)
      } else if (noteID !== null) {
        activeSessionNote = await loadSessionNote(session, noteID)
        if (activeSessionNote === null) {
          navigate(sessionNotesPath(activeWorkspace, session), true)
        }
      }
    } else {
      await loadSessionEvents(session)
    }
    startActivityPolling()
  }

  async function selectSessionNotes(session: Session) {
    if (activeWorkspace === null || activeSession?.id !== session.id) {
      return
    }
    mobileMenuOpen = false
    activeSessionNote = null
    creatingSessionNote = false
    editingSessionNote = false
    sessionNoteError = ""
    navigate(sessionNotesPath(activeWorkspace, session), false)
    await loadSessionNotes(session)
    startActivityPolling()
  }

  async function selectSessionChat(session: Session) {
    if (activeWorkspace === null || activeSession?.id !== session.id) {
      return
    }
    navigate(sessionPath(activeWorkspace, session), false)
    await selectSession(session)
  }

  async function selectProject(project: Project, replace = false) {
    if (activeWorkspace === null) {
      return
    }
    mobileMenuOpen = false
    activeSession = null
    activeProject = project
    activeProjectNote = null
    creatingProjectNote = false
    projectFiles = []
    projectFileError = ""
    projectNotes = []
    events = []
    navigate(`${projectsPath(activeWorkspace)}/${encodeURIComponent(project.id)}`, replace)
    await Promise.all([loadProjectSessions(project), loadProjectFiles(project), loadProjectNotes(project)])
    startActivityPolling()
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

  async function loadProjectSessions(project: Project) {
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
    if (activeWorkspace?.id === workspace.id && activeProject?.id === project.id) {
      projectSessions = loaded.sessions
    }
  }

  async function loadProjectFiles(project: Project, showLoading = true) {
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
      if (activeWorkspace?.id !== workspace.id || activeProject?.id !== project.id || activeSession !== null) {
        return false
      }
      projectFiles = [...loaded].sort((left, right) => {
        const difference = new Date(right.created_at).getTime() - new Date(left.created_at).getTime()
        return Number.isFinite(difference) && difference !== 0 ? difference : right.id.localeCompare(left.id)
      })
      projectFileStatus = "ready"
      return true
    } catch {
      if (activeWorkspace?.id === workspace.id && activeProject?.id === project.id && activeSession === null) {
        projectFileStatus = "unavailable"
      }
      return false
    }
  }

  async function loadProjectNotes(project: Project, showLoading = true) {
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
      if (activeWorkspace?.id !== workspace.id || activeProject?.id !== project.id || activeSession !== null) {
        return false
      }
      projectNotes = [...loaded].sort((left, right) => {
        const difference = new Date(right.created_at).getTime() - new Date(left.created_at).getTime()
        return Number.isFinite(difference) && difference !== 0 ? difference : right.id.localeCompare(left.id)
      })
      projectNoteStatus = "ready"
      return true
    } catch {
      if (activeWorkspace?.id === workspace.id && activeProject?.id === project.id && activeSession === null) {
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

  async function loadSessionNotes(session: Session, showLoading = true) {
    if (activeWorkspace === null || !isSessionNotesRoute()) {
      return false
    }
    const workspace = activeWorkspace
    if (showLoading) {
      sessionNoteStatus = "checking"
    }
    try {
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions/${encodeURIComponent(session.id)}/notes`, { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return false
      }
      if (!response.ok) {
        throw new Error("session notes could not be loaded")
      }
      const loaded = (await response.json()) as SessionNote[]
      if (activeWorkspace?.id !== workspace.id || activeSession?.id !== session.id || !isSessionNotesRoute()) {
        return false
      }
      sessionNotes = [...loaded].sort((left, right) => {
        const difference = new Date(right.created_at).getTime() - new Date(left.created_at).getTime()
        return Number.isFinite(difference) && difference !== 0 ? difference : right.id.localeCompare(left.id)
      })
      sessionNoteStatus = "ready"
      return true
    } catch {
      if (activeWorkspace?.id === workspace.id && activeSession?.id === session.id && isSessionNotesRoute()) {
        sessionNoteStatus = "unavailable"
      }
      return false
    }
  }

  async function loadSessionNote(session: Session, id: string) {
    if (activeWorkspace === null || !isSessionNotesRoute()) {
      return null
    }
    const workspace = activeWorkspace
    const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions/${encodeURIComponent(session.id)}/notes/${encodeURIComponent(id)}`, { credentials: "same-origin" })
    if (response.status === 401) {
      signInRequired()
      return null
    }
    if (response.status === 404) {
      return null
    }
    if (!response.ok) {
      throw new Error("session note could not be loaded")
    }
    return (await response.json()) as SessionNote
  }

  async function loadSessionEvents(session: Session, showLoading = true) {
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
      if (activeSession?.id !== session.id) {
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
      if (activeSession?.id === session.id) {
        eventStatus = "unavailable"
      }
      return false
    }
  }

  function stopActivityPolling(clearCursors = true) {
    activityPollGeneration += 1
    if (activityPollTimer !== undefined) {
      clearTimeout(activityPollTimer)
      activityPollTimer = undefined
    }
    if (clearCursors) {
      activeSessionCursor = null
      activeProjectCursor = null
      workspaceSessionsCursor = null
      workspaceProjectsCursor = null
    }
    awaitingReplyFor = []
  }

  async function refreshWorkspaceSessions(workspace: Workspace, generation: number) {
    const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions?limit=5`, { credentials: "same-origin" })
    if (response.status === 401) {
      signInRequired()
      return false
    }
    if (!response.ok) {
      throw new Error("sessions could not be refreshed")
    }
    const loaded = (await response.json()) as SessionSearchResponse
    if (generation !== activityPollGeneration || activeWorkspace?.id !== workspace.id) {
      return false
    }
    latestSessions = loaded.sessions
    if (activeSession !== null) {
      activeSession = loaded.sessions.find((session) => session.id === activeSession?.id) ?? activeSession
    }
    return true
  }

  async function refreshWorkspaceProjects(workspace: Workspace, generation: number) {
    const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects?limit=5`, { credentials: "same-origin" })
    if (response.status === 401) {
      signInRequired()
      return false
    }
    if (!response.ok) {
      throw new Error("projects could not be refreshed")
    }
    const loaded = (await response.json()) as ProjectSearchResponse
    if (generation !== activityPollGeneration || activeWorkspace?.id !== workspace.id) {
      return false
    }
    latestProjects = loaded.projects
    if (activeProject !== null) {
      activeProject = loaded.projects.find((project) => project.id === activeProject?.id) ?? activeProject
    }
    return true
  }

  function sameActivityCursor(left: ActivityCursor | null, right: ActivityCursor | null) {
    return left?.id === right?.id
  }

  function startActivityPolling() {
    if (activeWorkspace === null || activityPollTimer !== undefined) {
      return
    }
    const workspace = activeWorkspace
    const generation = activityPollGeneration
    const poll = async () => {
      if (generation !== activityPollGeneration || activeWorkspace?.id !== workspace.id) {
        return
      }
      try {
        const session = activeSession
        const overview = session === null && activeProject === null
        const sessionTopic = session === null ? (overview ? "session/*" : undefined) : `session/${session.id}`
        const projectID = session?.project?.id ?? activeProject?.id
        const projectTopic = projectID === undefined ? (overview ? "project/*" : undefined) : `project/${projectID}`
        if (sessionTopic === undefined && projectTopic === undefined) {
          return
        }
        const topics: { topic: string; cursor: ActivityCursor | null }[] = []
        if (sessionTopic !== undefined) {
          topics.push({ topic: sessionTopic, cursor: session === null ? workspaceSessionsCursor : activeSessionCursor })
        }
        if (projectTopic !== undefined) {
          topics.push({ topic: projectTopic, cursor: projectID === undefined ? workspaceProjectsCursor : activeProjectCursor })
        }
        const input: ActivityTopicCheckpoints = {
          topics,
        }
        const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/activity`, {
          method: "POST",
          credentials: "same-origin",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(input),
        })
        if (response.status === 401) {
          signInRequired()
          return
        }
        if (!response.ok) {
          throw new Error("activity checkpoints could not be loaded")
        }
        const output = (await response.json()) as ActivityTopicCheckpoints
        if (generation !== activityPollGeneration || activeWorkspace?.id !== workspace.id) {
          return
        }
        const returned = new Map(output.topics.map((checkpoint) => [checkpoint.topic, checkpoint.cursor ?? null]))
        const nextSessionCursor = sessionTopic === undefined ? null : returned.get(sessionTopic) ?? null
        const nextProjectCursor = projectTopic === undefined ? null : returned.get(projectTopic) ?? null
        const sessionCursor = session === null ? workspaceSessionsCursor : activeSessionCursor
        const projectCursor = projectID === undefined ? workspaceProjectsCursor : activeProjectCursor
        const sessionChanged = sessionTopic !== undefined && !sameActivityCursor(sessionCursor, nextSessionCursor)
        const projectChanged = projectTopic !== undefined && !sameActivityCursor(projectCursor, nextProjectCursor)
        let refreshed = !sessionChanged && !projectChanged || (await Promise.all([
          refreshWorkspaceSessions(workspace, generation),
          ...(projectChanged ? [refreshWorkspaceProjects(workspace, generation)] : []),
        ])).every(Boolean)
        if (refreshed && session !== null && (sessionChanged || projectChanged) && activeSession?.id === session.id) {
          if (isSessionNotesRoute()) {
            if (sessionChanged) {
              refreshed = await loadSessionNotes(session, false)
              if (refreshed && activeSessionNote !== null) {
                const note = activeSessionNote
                const loaded = await loadSessionNote(session, note.id)
                if (loaded === null && activeWorkspace?.id === workspace.id && activeSession?.id === session.id && activeSessionNote?.id === note.id) {
                  activeSessionNote = null
                  editingSessionNote = false
                  navigate(sessionNotesPath(workspace, session))
                } else if (loaded !== null && activeSessionNote?.id === note.id) {
                  activeSessionNote = loaded
                }
              }
            }
          } else {
            await loadSessionEvents(session, false)
          }
        }
        if (refreshed && session === null && projectChanged && activeProject !== null && activeProject.id === projectID) {
          refreshed = (await Promise.all([loadProjectFiles(activeProject, false), loadProjectNotes(activeProject, false)])).every(Boolean)
          if (refreshed && activeProjectNote !== null) {
            const note = activeProjectNote
            const loaded = await loadProjectNote(activeProject, note.id)
            if (loaded === null && activeWorkspace?.id === workspace.id && activeProject?.id === projectID && activeProjectNote?.id === note.id) {
              activeProjectNote = null
              editingProjectNote = false
              navigate(`${projectsPath(workspace)}/${encodeURIComponent(activeProject.id)}`)
            } else if (loaded !== null && activeProjectNote?.id === note.id) {
              activeProjectNote = loaded
            }
          }
        }
        if (generation !== activityPollGeneration || activeWorkspace?.id !== workspace.id) {
          return
        }
        if (refreshed && sessionChanged) {
          if (session === null) {
            workspaceSessionsCursor = nextSessionCursor
          } else {
            activeSessionCursor = nextSessionCursor
          }
        }
        if (refreshed && projectChanged) {
          if (projectID === undefined) {
            workspaceProjectsCursor = nextProjectCursor
          } else {
            activeProjectCursor = nextProjectCursor
          }
        }
      } catch {
        // Keep checkpoints unchanged so a transient failure retries the same invalidation.
      } finally {
        if (generation === activityPollGeneration && activeWorkspace?.id === workspace.id) {
          activityPollTimer = setTimeout(poll, 1000)
        }
      }
    }
    activityPollTimer = setTimeout(poll, 1000)
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

  function elapsedDuration(startedAt: string, completedAt: string) {
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
      await selectSession(session)
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
      await selectProject(project)
    } catch {
      messageError = "A new project could not be created. Try again."
    } finally {
      creatingProject = false
    }
  }

  async function selectProjectNote(note: ProjectNote) {
    if (activeWorkspace === null || activeProject === null) {
      return
    }
    const workspace = activeWorkspace
    const project = activeProject
    projectNoteError = ""
    creatingProjectNote = false
    editingProjectNote = false
    navigate(projectNotePath(workspace, project, note), false)
    try {
      const loaded = await loadProjectNote(project, note.id)
      if (activeWorkspace?.id !== workspace.id || activeProject?.id !== project.id) {
        return
      }
      if (loaded === null) {
        navigate(`${projectsPath(workspace)}/${encodeURIComponent(project.id)}`)
        return
      }
      activeProjectNote = loaded
    } catch {
      projectNoteError = "The note could not be loaded. Try again."
    }
  }

  function startProjectNoteCreate() {
    if (activeWorkspace === null || activeProject === null) {
      return
    }
    activeProjectNote = null
    creatingProjectNote = true
    editingProjectNote = true
    projectNoteTitle = ""
    projectNoteDescription = ""
    projectNoteBody = ""
    projectNoteError = ""
    navigate(projectNotePath(activeWorkspace, activeProject, "new"), false)
  }

  function startProjectNoteEdit() {
    if (activeProjectNote === null) {
      return
    }
    projectNoteTitle = activeProjectNote.title
    projectNoteDescription = activeProjectNote.description
    projectNoteBody = activeProjectNote.body ?? ""
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
      const path = creating ? `/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects/${encodeURIComponent(project.id)}/notes` : `/api/v1/workspaces/${encodeURIComponent(workspace.id)}/projects/${encodeURIComponent(project.id)}/notes/${encodeURIComponent(note?.id ?? "")}`
      const response = await fetch(path, {
        method: creating ? "POST" : "PATCH",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ title: projectNoteTitle, description: projectNoteDescription, body: projectNoteBody }),
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

  async function selectSessionNote(note: SessionNote) {
    if (activeWorkspace === null || activeSession === null) {
      return
    }
    const workspace = activeWorkspace
    const session = activeSession
    sessionNoteError = ""
    creatingSessionNote = false
    editingSessionNote = false
    navigate(sessionNotePath(workspace, session, note), false)
    try {
      const loaded = await loadSessionNote(session, note.id)
      if (activeWorkspace?.id !== workspace.id || activeSession?.id !== session.id) {
        return
      }
      if (loaded === null) {
        navigate(sessionNotesPath(workspace, session))
        return
      }
      activeSessionNote = loaded
    } catch {
      sessionNoteError = "The note could not be loaded. Try again."
    }
  }

  function startSessionNoteCreate(navigateRoute = true) {
    if (activeWorkspace === null || activeSession === null) {
      return
    }
    activeSessionNote = null
    creatingSessionNote = true
    editingSessionNote = true
    sessionNoteTitle = ""
    sessionNoteDescription = ""
    sessionNoteBody = ""
    sessionNoteError = ""
    if (navigateRoute) {
      navigate(sessionNotePath(activeWorkspace, activeSession, "new"), false)
    }
  }

  function startSessionNoteEdit() {
    if (activeSessionNote === null) {
      return
    }
    sessionNoteTitle = activeSessionNote.title
    sessionNoteDescription = activeSessionNote.description
    sessionNoteBody = activeSessionNote.body ?? ""
    sessionNoteError = ""
    editingSessionNote = true
  }

  function cancelSessionNoteEdit() {
    if (savingSessionNote || activeWorkspace === null || activeSession === null) {
      return
    }
    sessionNoteError = ""
    if (creatingSessionNote) {
      creatingSessionNote = false
      editingSessionNote = false
      navigate(sessionNotesPath(activeWorkspace, activeSession))
      return
    }
    editingSessionNote = false
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
        body: JSON.stringify({ title: sessionNoteTitle, description: sessionNoteDescription, body: sessionNoteBody }),
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
      creatingSessionNote = false
      editingSessionNote = false
      sessionNotes = [saved, ...sessionNotes.filter((candidate) => candidate.id !== saved.id)]
      navigate(sessionNotePath(workspace, session, saved))
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

  async function login() {
    loginError = ""
    submitting = true
    try {
      const response = await fetch("/api/v1/auth/login", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ identity, password }),
      })
      if (response.status === 401) {
        loginError = "The username or password is incorrect."
        return
      }
      if (!response.ok) {
        throw new Error(`login returned ${response.status}`)
      }
      password = ""
      await checkSession(true)
    } catch {
      loginError = "Gatehouse could not be reached. Try again."
    } finally {
      submitting = false
    }
  }

  async function logout() {
    try {
      await fetch("/api/v1/auth/logout", { method: "POST", credentials: "same-origin" })
    } finally {
      signInRequired()
    }
  }
</script>

<svelte:head>
  <meta name="description" content="Gatehouse hosted chat" />
  <title>Gatehouse</title>
</svelte:head>

{#if status === "checking" || (status === "authenticated" && workspaceStatus === "checking")}
  <main class="auth-shell" aria-busy="true" aria-live="polite">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <div class="loading-mark" aria-hidden="true"></div>
      <p>{status === "checking" ? "Checking your session." : "Loading your workspaces."}</p>
    </section>
  </main>
{:else if status === "unavailable" || workspaceStatus === "unavailable"}
  <main class="auth-shell">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">Connection unavailable</h1>
      <p class="subtitle is-6">Gatehouse could not load your account.</p>
      <button class="button is-primary" type="button" onclick={() => void checkSession()}>Try again</button>
    </section>
  </main>
{:else if status === "anonymous"}
  <main class="auth-shell">
    <section class="login-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-2">Welcome back.</h1>
      <p class="subtitle is-6">Sign in to continue to your workspace.</p>
      <form autocomplete="off" onsubmit={(event) => { event.preventDefault(); void login() }}>
        <div class="field">
          <label class="label" for="identity">Username</label>
          <div class="control">
            <input class="input" id="identity" name="identity" autocomplete="username" required bind:value={identity} />
          </div>
        </div>
        <div class="field">
          <label class="label" for="password">Password</label>
          <div class="control">
            <input class="input" id="password" name="password" type="password" autocomplete="current-password" required bind:value={password} />
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
{:else if workspaceStatus === "empty"}
  <main class="auth-shell">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">No workspace access</h1>
      <p class="subtitle is-6">Ask an administrator to add {claims?.principal.name ?? "User"} to a workspace group.</p>
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
              void selectWorkspace(workspace)
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
            <li><a class:active={isChatCollection()} href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { void selectWorkspaceRoute(sessionsPath(activeWorkspace)) } }}>Chats</a></li>
            <li><a class:active={isProjectCollection()} href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/prj`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { void selectWorkspaceRoute(projectsPath(activeWorkspace)) } }}>Projects</a></li>
            <li><a class:active={isGroupCollection()} href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/grp`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { void selectWorkspaceRoute(groupsPath(activeWorkspace)) } }}>Groups</a></li>
          </ul>
        </section>
      </nav>

      <div class="sidebar-footer">
        <span>{claims?.principal.name ?? "User"}</span>
        <button class="button is-small is-danger is-light" type="button" onclick={() => void logout()}>Log out</button>
      </div>
    </aside>

    <main class="workspace-main" bind:this={workspaceMainElement} onscroll={trackChatScroll}>
      <header class="workspace-header">
          <button class="mobile-menu-trigger" type="button" aria-label="Open navigation menu" aria-expanded={mobileMenuOpen} onclick={() => mobileMenuOpen = true}>
            <Menu size={20} strokeWidth={2} aria-hidden="true" />
          </button>
          <h1 class="workspace-breadcrumb">
            <a href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { void selectWorkspaceRoute(workspacePath(activeWorkspace)) } }}>{activeWorkspace?.name ?? "New Workspace"}</a>
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
              {#if activeSession === null && activeProjectNote === null && !creatingProjectNote}
                <span>{activeProject.name ?? "New Project"}</span>
              {:else}
                <a href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/prj/${encodeURIComponent(activeProject.id)}`} onclick={(event) => { event.preventDefault(); void selectProject(activeProject) }}>{activeProject.name ?? "New Project"}</a>
              {/if}
            {/if}
            {#if activeSession !== null}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              {#if isSessionNotesRoute()}
                <a href={activeWorkspace !== null ? sessionPath(activeWorkspace, activeSession) : "#"} onclick={(event) => { event.preventDefault(); void selectSessionChat(activeSession) }}>{activeSession.name ?? "New Chat"}</a>
              {:else}
                <span>{activeSession.name ?? "New Chat"}</span>
              {/if}
            {:else if activeProjectNote !== null || creatingProjectNote}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              <span>{activeProjectNote?.title ?? "New Note"}</span>
            {/if}
            {#if activeSession !== null && (activeSessionNote !== null || creatingSessionNote)}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              <span>{activeSessionNote?.title ?? "New Note"}</span>
            {/if}
          </h1>
      </header>
      {#if activeSession !== null}
        <nav class="session-tabs" aria-label="Session navigation">
          <a class:active={!isSessionNotesRoute()} href={activeWorkspace !== null ? sessionPath(activeWorkspace, activeSession) : "#"} onclick={(event) => { event.preventDefault(); void selectSessionChat(activeSession) }}>Chat</a>
          <a class:active={isSessionNotesRoute()} href={activeWorkspace !== null ? sessionNotesPath(activeWorkspace, activeSession) : "#"} onclick={(event) => { event.preventDefault(); void selectSessionNotes(activeSession) }}>Notes</a>
        </nav>
      {/if}
      {#if activeSession === null && activeProject !== null && activeProjectNote === null && !creatingProjectNote}
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
              <a class="dashboard-row" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses/${encodeURIComponent(session.id)}`} onclick={(event) => { event.preventDefault(); void selectSession(session) }}><span class="dashboard-row-content"><span>{session.name ?? "New Chat"}</span><span class="dashboard-row-meta"><time datetime={session.created_at}>{createdAtLabel(session.created_at)}</time>{#if session.project !== undefined}<span aria-hidden="true">/</span><span>{session.project.name ?? "New Project"}</span>{/if}</span></span></a>
            {:else}<p class="dashboard-empty">No chats yet.</p>{/each}
            {#if latestSessions.length > 0}<a class="dashboard-view-all" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { void selectWorkspaceRoute(sessionsPath(activeWorkspace)) } }}>View all chats</a>{/if}
          </section>
          <section class="dashboard-widget dashboard-widget-wide">
            <div class="dashboard-widget-heading"><h2>Latest Projects</h2><button class="button is-primary is-small" type="button" disabled={creatingProject} onclick={() => void createProject()}>New project</button></div>
            {#each latestProjects as project}
              <a class="dashboard-row" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/prj/${encodeURIComponent(project.id)}`} onclick={(event) => { event.preventDefault(); void selectProject(project) }}><span class="dashboard-row-content"><span>{project.name ?? "New Project"}</span><time datetime={project.created_at}>{createdAtLabel(project.created_at)}</time></span></a>
            {:else}<p class="dashboard-empty">No projects yet.</p>{/each}
            {#if latestProjects.length > 0}<a class="dashboard-view-all" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/prj`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { void selectWorkspaceRoute(projectsPath(activeWorkspace)) } }}>View all projects</a>{/if}
          </section>
          {#if messageError !== ""}<p class="help is-danger dashboard-error" aria-live="polite">{messageError}</p>{/if}
        </section>
      {:else if isChatCollection()}
        <section class="collection-page">
          <div class="collection-heading"><h2>Chats</h2><button class="button is-primary is-small" type="button" onclick={() => void createSession()}>New chat</button></div>
          <form class="collection-search" onsubmit={(event) => { event.preventDefault(); void submitSessionSearch() }}>
            <label><span>Search chats</span><input class="input" type="search" placeholder="Search chats" bind:value={chatSearch} /></label>
            <button class="button" type="submit" aria-label="Search chats" title="Search chats"><Search size={20} strokeWidth={2} aria-hidden="true" /></button>
          </form>
          <div class="collection-list">
            {#each searchedSessions as session}
              <a class="dashboard-row" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses/${encodeURIComponent(session.id)}`} onclick={(event) => { event.preventDefault(); void selectSession(session) }}><span class="dashboard-row-content"><span>{session.name ?? "New Chat"}</span><span class="dashboard-row-meta"><time datetime={session.created_at}>{createdAtLabel(session.created_at)}</time>{#if session.project !== undefined}<span aria-hidden="true">/</span><span>{session.project.name ?? "New Project"}</span>{/if}</span></span></a>
            {:else}<p class="dashboard-empty">{sessionSearchLoading ? "Searching chats..." : "No chats match your search."}</p>{/each}
          </div>
          {#if sessionSearchCursor !== null}<button class="button is-small" type="button" disabled={sessionSearchLoading} onclick={() => void loadSessionSearch()}>{sessionSearchLoading ? "Loading..." : "Show more"}</button>{/if}
        </section>
      {:else if isProjectCollection()}
        <section class="collection-page">
          <div class="collection-heading"><h2>Projects</h2><button class="button is-primary is-small" type="button" disabled={creatingProject} onclick={() => void createProject()}>New project</button></div>
          <form class="collection-search" onsubmit={(event) => { event.preventDefault(); void submitProjectSearch() }}>
            <label><span>Search projects</span><input class="input" type="search" placeholder="Search projects" bind:value={projectSearch} /></label>
            <button class="button" type="submit" aria-label="Search projects" title="Search projects"><Search size={20} strokeWidth={2} aria-hidden="true" /></button>
          </form>
          <div class="collection-list">
            {#each searchedProjects as project}
              <a class="dashboard-row" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/prj/${encodeURIComponent(project.id)}`} onclick={(event) => { event.preventDefault(); void selectProject(project) }}><span class="dashboard-row-content"><span>{project.name ?? "New Project"}</span><time datetime={project.created_at}>{createdAtLabel(project.created_at)}</time></span></a>
            {:else}<p class="dashboard-empty">{projectSearchLoading ? "Searching projects..." : "No projects match your search."}</p>{/each}
          </div>
          {#if projectSearchCursor !== null}<button class="button is-small" type="button" disabled={projectSearchLoading} onclick={() => void loadProjectSearch()}>{projectSearchLoading ? "Loading..." : "Show more"}</button>{/if}
        </section>
      {:else if isGroupCollection()}
        <section class="collection-page">
          <div class="collection-heading"><h2>Groups</h2></div>
          <div class="collection-search"><label><span>Search groups</span><input class="input" type="search" placeholder="Search groups" bind:value={groupSearch} /></label></div>
          <div class="collection-list">
            {#each ordered(groups.filter((group) => matchesSearch(group, groupSearch))) as group}
              <div class="dashboard-row"><span>{group.name ?? "New Group"}</span><small>{group.id}</small></div>
            {:else}<p class="dashboard-empty">No groups match your search.</p>{/each}
          </div>
        </section>
      {:else if activeSession === null && (activeProjectNote !== null || creatingProjectNote)}
        <section class="project-note-page">
          {#if editingProjectNote}
            <form class="project-note-editor" onsubmit={(event) => { event.preventDefault(); void saveProjectNote() }}>
              <div class="project-note-page-heading"><div><p class="eyebrow">Project Note</p><h2>{creatingProjectNote ? "New Note" : "Edit Note"}</h2></div></div>
              <div class="field"><label class="label" for="project-note-title">Title</label><div class="control"><input class="input" id="project-note-title" maxlength="256" required bind:value={projectNoteTitle} /></div></div>
              <div class="field"><label class="label" for="project-note-description">Description (optional)</label><div class="control"><textarea class="textarea" id="project-note-description" rows="3" maxlength="4096" bind:value={projectNoteDescription}></textarea></div></div>
              <div class="field"><label class="label" for="project-note-body">Content (optional)</label><div class="control"><textarea class="textarea project-note-body-input" id="project-note-body" rows="18" maxlength="1048576" bind:value={projectNoteBody}></textarea></div></div>
              {#if projectNoteError !== ""}<p class="help is-danger" aria-live="polite">{projectNoteError}</p>{/if}
              <div class="project-note-actions"><button class="button" type="button" disabled={savingProjectNote} onclick={cancelProjectNoteEdit}>Cancel</button><button class="button is-primary" type="submit" disabled={savingProjectNote}>{savingProjectNote ? "Saving..." : "Save note"}</button></div>
            </form>
          {:else if activeProjectNote !== null}
            <article class="project-note-view">
              <header class="project-note-page-heading"><div><p class="eyebrow">Project Note</p><h2>{activeProjectNote.title}</h2>{#if activeProjectNote.description !== ""}<p>{activeProjectNote.description}</p>{/if}<small>By {activeProjectNote.author.name ?? activeProjectNote.author.id} on {createdAtLabel(activeProjectNote.created_at)}</small></div><div class="project-note-actions"><button class="button is-small" type="button" onclick={startProjectNoteEdit}>Edit</button><button class="button is-small is-danger is-light" type="button" disabled={deletingProjectNote} onclick={() => void removeProjectNote()}>{deletingProjectNote ? "Removing..." : "Remove"}</button></div></header>
              {#if activeProjectNote.body !== undefined && activeProjectNote.body !== ""}<div class="project-note-markdown">{@html renderMarkdown(activeProjectNote.body)}</div>{/if}
              {#if projectNoteError !== ""}<p class="help is-danger" aria-live="polite">{projectNoteError}</p>{/if}
            </article>
          {/if}
        </section>
      {:else if activeSession !== null && isSessionNotesRoute()}
        <section class="project-note-page">
          {#if editingSessionNote}
            <form class="project-note-editor" onsubmit={(event) => { event.preventDefault(); void saveSessionNote() }}>
              <div class="project-note-page-heading"><div><p class="eyebrow">Session Note</p><h2>{creatingSessionNote ? "New Note" : "Edit Note"}</h2></div></div>
              <div class="field"><label class="label" for="session-note-title">Title</label><div class="control"><input class="input" id="session-note-title" maxlength="256" required bind:value={sessionNoteTitle} /></div></div>
              <div class="field"><label class="label" for="session-note-description">Description (optional)</label><div class="control"><textarea class="textarea" id="session-note-description" rows="3" maxlength="4096" bind:value={sessionNoteDescription}></textarea></div></div>
              <div class="field"><label class="label" for="session-note-body">Content (optional)</label><div class="control"><textarea class="textarea project-note-body-input" id="session-note-body" rows="18" maxlength="1048576" bind:value={sessionNoteBody}></textarea></div></div>
              {#if sessionNoteError !== ""}<p class="help is-danger" aria-live="polite">{sessionNoteError}</p>{/if}
              <div class="project-note-actions"><button class="button" type="button" disabled={savingSessionNote} onclick={cancelSessionNoteEdit}>Cancel</button><button class="button is-primary" type="submit" disabled={savingSessionNote}>{savingSessionNote ? "Saving..." : "Save note"}</button></div>
            </form>
          {:else if activeSessionNote !== null}
            <article class="project-note-view">
              <header class="project-note-page-heading"><div><p class="eyebrow">Session Note</p><h2>{activeSessionNote.title}</h2>{#if activeSessionNote.description !== ""}<p>{activeSessionNote.description}</p>{/if}<small>By {activeSessionNote.author.name ?? activeSessionNote.author.id} on {createdAtLabel(activeSessionNote.created_at)}</small></div><div class="project-note-actions"><button class="button is-small" type="button" onclick={startSessionNoteEdit}>Edit</button><button class="button is-small is-danger is-light" type="button" disabled={deletingSessionNote} onclick={() => void removeSessionNote()}>{deletingSessionNote ? "Removing..." : "Remove"}</button></div></header>
              {#if activeSessionNote.body !== undefined && activeSessionNote.body !== ""}<div class="project-note-markdown">{@html renderMarkdown(activeSessionNote.body)}</div>{/if}
              {#if sessionNoteError !== ""}<p class="help is-danger" aria-live="polite">{sessionNoteError}</p>{/if}
            </article>
          {:else}
            <div class="collection-heading"><h2>Notes</h2><button class="button is-primary is-small" type="button" onclick={() => startSessionNoteCreate()}>New note</button></div>
            <div class="collection-list">
              {#if sessionNoteStatus === "checking"}
                <p class="dashboard-empty">Loading notes...</p>
              {:else if sessionNoteStatus === "unavailable"}
                <p class="dashboard-empty">Notes could not be loaded.</p>
              {:else}
                {#each sessionNotes as note (note.id)}
                  <a class="dashboard-row project-note-row" href={activeWorkspace !== null && activeSession !== null ? sessionNotePath(activeWorkspace, activeSession, note) : "#"} onclick={(event) => { event.preventDefault(); void selectSessionNote(note) }}><span class="dashboard-row-content"><span>{note.title}</span>{#if note.description !== ""}<span class="project-note-description">{note.description}</span>{/if}<span class="dashboard-row-meta"><span>{note.author.name ?? note.author.id}</span><time datetime={note.created_at}>{createdAtLabel(note.created_at)}</time></span></span></a>
                {:else}<p class="dashboard-empty">No notes yet.</p>{/each}
              {/if}
            </div>
          {/if}
        </section>
      {:else if activeSession === null}
        <section class="dashboard-grid">
          <section class="dashboard-widget dashboard-widget-wide">
            <div class="dashboard-widget-heading"><h2>Project Chats</h2><button class="button is-primary is-small" type="button" onclick={() => void createSession(activeProject ?? undefined)}>New chat</button></div>
            {#each projectSessions as session}
              <a class="dashboard-row" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses/${encodeURIComponent(session.id)}`} onclick={(event) => { event.preventDefault(); void selectSession(session) }}><span class="dashboard-row-content"><span>{session.name ?? "New Chat"}</span><span class="dashboard-row-meta"><time datetime={session.created_at}>{createdAtLabel(session.created_at)}</time>{#if session.project !== undefined}<span aria-hidden="true">/</span><span>{session.project.name ?? "New Project"}</span>{/if}</span></span></a>
            {:else}<p class="dashboard-empty">No project chats yet.</p>{/each}
            {#if projectSessions.length > 0}<a class="dashboard-view-all" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { void selectWorkspaceRoute(sessionsPath(activeWorkspace)) } }}>View all chats</a>{/if}
          </section>
          <section class="dashboard-widget dashboard-widget-wide project-files-widget">
            <div class="dashboard-widget-heading"><h2>Project Files</h2><button class="button is-primary is-small" type="button" disabled={uploadingProjectFiles > 0} onclick={() => projectFileInputElement?.click()}>{uploadingProjectFiles > 0 ? "Uploading..." : "Upload files"}</button></div>
            <input class="is-sr-only" type="file" multiple bind:this={projectFileInputElement} onchange={(event) => void uploadProjectFiles(event.currentTarget)} />
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
                <a class="dashboard-row project-note-row" href={activeWorkspace !== null && activeProject !== null ? projectNotePath(activeWorkspace, activeProject, note) : "#"} onclick={(event) => { event.preventDefault(); void selectProjectNote(note) }}><span class="dashboard-row-content"><span>{note.title}</span>{#if note.description !== ""}<span class="project-note-description">{note.description}</span>{/if}<span class="dashboard-row-meta"><time datetime={note.created_at}>{createdAtLabel(note.created_at)}</time></span></span></a>
              {:else}<p class="dashboard-empty">No notes yet.</p>{/each}
            {/if}
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
                      <div class="chat-message-text">{@html renderMarkdown(tree.event.payload.text)}</div>
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
                        {hasCancellationSuccess(tree) ? "Cancelled" : cancellationRequest(tree) !== undefined ? "Cancellation requested" : finalReplies(tree).length === 0 ? "Agent is working" : "Agent activity"}
                        {#if finalReplies(tree).length > 0 && replyDuration(tree) !== ""}
                          <span class="agent-activity-duration">{replyDuration(tree)}</span>
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
                      <button class="chat-message-copy" type="button" aria-label="Copy response Markdown" title="Copy Markdown" onclick={() => void copyMarkdown(reply.event.payload.text)}>
                        <Copy size={16} strokeWidth={2} />
                      </button>
                      <div class="chat-message-text">{@html renderMarkdown(reply.event.payload.text)}</div>
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
            <input class="is-sr-only" id="files" type="file" multiple bind:this={fileInputElement} onchange={(event) => selectComposerFiles(event.currentTarget)} />
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
  <dialog class="project-edit-dialog" bind:this={projectEditDialogElement} onclose={() => projectEditError = ""}>
    <form class="project-edit-form" onsubmit={(event) => { event.preventDefault(); void updateProject() }}>
      <div class="project-edit-heading"><h2>Edit project</h2><button class="button is-ghost is-small" type="button" aria-label="Close" onclick={closeProjectEdit}><X size={18} strokeWidth={2} aria-hidden="true" /></button></div>
      <div class="field">
        <label class="label" for="project-edit-name">Name</label>
        <div class="control"><input class="input" id="project-edit-name" maxlength="256" bind:value={projectEditName} /></div>
      </div>
      <div class="field">
        <label class="label" for="project-edit-description">Description</label>
        <div class="control"><textarea class="textarea" id="project-edit-description" rows="4" maxlength="4096" bind:value={projectEditDescription}></textarea></div>
      </div>
      {#if projectEditError !== ""}<p class="help is-danger" aria-live="polite">{projectEditError}</p>{/if}
      <div class="project-edit-actions"><button class="button" type="button" disabled={updatingProject} onclick={closeProjectEdit}>Cancel</button><button class="button is-primary" type="submit" disabled={updatingProject}>{updatingProject ? "Saving..." : "Save changes"}</button></div>
    </form>
  </dialog>
{/if}
