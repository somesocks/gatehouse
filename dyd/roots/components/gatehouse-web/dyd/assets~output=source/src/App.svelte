<script lang="ts">
  import { onMount, tick } from "svelte"
  import { Bot, CircleCheck, CircleX, Copy, Menu, Paperclip, Search, Send, X } from "@lucide/svelte"
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
    payload: { text?: string; name?: string; reason?: string; code?: string; output?: string; files?: MessageFile[] }
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
  let creatingProject = $state(false)
  let events = $state<SessionEventTree[]>([])
  let eventStatus = $state<WorkspaceContentStatus>("checking")
  let messageText = $state("")
  let composerFiles = $state<ComposerFile[]>([])
  let messageError = $state("")
  let sendingMessage = $state(false)
  let awaitingReplyFor = $state<string[]>([])
  let cancellingReplyFor = $state<Set<string>>(new Set())
  let expandedActivity = $state<Set<string>>(new Set())
  let mobileMenuOpen = $state(false)
  let showJumpToLatest = $state(false)
  let chatEventsElement = $state<HTMLDivElement | undefined>()
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
    window.addEventListener("popstate", handlePopState)
    void checkSession()
    return () => {
      window.removeEventListener("popstate", handlePopState)
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

  function workspacePath(workspace: Workspace) {
    return `/app/wsp/${encodeURIComponent(workspace.id)}`
  }

  function sessionsPath(workspace: Workspace) {
    return `${workspacePath(workspace)}/ses`
  }

  function projectsPath(workspace: Workspace) {
    return `${workspacePath(workspace)}/prj`
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
    activeProject = null
    projectFiles = []
    projectFileError = ""
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
    activeProject = null
    projectFiles = []
    projectFileError = ""
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
          await Promise.all([loadProjectSessions(activeProject), loadProjectFiles(activeProject)])
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
    activeProject = session.project ?? null
    projectFiles = []
    projectFileError = ""
    events = []
    showJumpToLatest = false
    eventStatus = "checking"
    messageError = ""
    if (sessionIDFromPath(currentPath) !== session.id) {
      navigate(`${sessionsPath(activeWorkspace)}/${encodeURIComponent(session.id)}`, replace)
    }
    await loadSessionEvents(session)
    startActivityPolling()
  }

  async function selectProject(project: Project, replace = false) {
    if (activeWorkspace === null) {
      return
    }
    mobileMenuOpen = false
    activeSession = null
    activeProject = project
    projectFiles = []
    projectFileError = ""
    events = []
    navigate(`${projectsPath(activeWorkspace)}/${encodeURIComponent(project.id)}`, replace)
    await Promise.all([loadProjectSessions(project), loadProjectFiles(project)])
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
          await loadSessionEvents(session, false)
        }
        if (refreshed && session === null && projectChanged && activeProject !== null && activeProject.id === projectID) {
          refreshed = await loadProjectFiles(activeProject, false)
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

  async function copyMarkdown(text: string) {
    await navigator.clipboard.writeText(text)
  }

  function isNearChatBottom() {
    if (chatEventsElement === undefined) {
      return true
    }
    return chatEventsElement.scrollHeight - chatEventsElement.scrollTop - chatEventsElement.clientHeight < 64
  }

  async function scrollToLatest(behavior: ScrollBehavior = "smooth") {
    await tick()
    if (chatEventsElement === undefined) {
      return
    }
    chatEventsElement.scrollTo({ top: chatEventsElement.scrollHeight, behavior })
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
        body: JSON.stringify({ ...(messageText.trim() === "" ? {} : { text: messageText }), ...(selectedAgent === "" ? {} : { agent: selectedAgent }), ...(fileIDs.length === 0 ? {} : { files: fileIDs }) }),
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

    <main class="workspace-main">
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
              {#if activeSession === null}
                <span>{activeProject.name ?? "New Project"}</span>
              {:else}
                <a href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/prj/${encodeURIComponent(activeProject.id)}`} onclick={(event) => { event.preventDefault(); void selectProject(activeProject) }}>{activeProject.name ?? "New Project"}</a>
              {/if}
            {/if}
            {#if activeSession !== null}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              <span>{activeSession.name ?? "New Chat"}</span>
            {/if}
          </h1>
      </header>
      {#if activeSession === null && activeProject === null && !isChatCollection() && !isProjectCollection() && !isGroupCollection()}
        <section class="dashboard-grid">
          <section class="dashboard-widget dashboard-widget-wide">
            <div class="dashboard-widget-heading"><h2>Latest Chats</h2><button class="button is-primary is-small" type="button" onclick={() => void createSession()}>New chat</button></div>
            {#each latestSessions as session}
              <a class="dashboard-row" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses/${encodeURIComponent(session.id)}`} onclick={(event) => { event.preventDefault(); void selectSession(session) }}><span class="dashboard-row-content"><span>{session.name ?? "New Chat"}</span><span class="dashboard-row-meta"><time datetime={session.created_at}>{createdAtLabel(session.created_at)}</time>{#if session.project !== undefined}<span aria-hidden="true">/</span><span>{session.project.name ?? "New Project"}</span>{/if}</span></span></a>
            {:else}<p class="dashboard-empty">No chats yet.</p>{/each}
            <a class="dashboard-view-all" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { void selectWorkspaceRoute(sessionsPath(activeWorkspace)) } }}>View all chats</a>
          </section>
          <section class="dashboard-widget dashboard-widget-wide">
            <div class="dashboard-widget-heading"><h2>Latest Projects</h2><button class="button is-primary is-small" type="button" disabled={creatingProject} onclick={() => void createProject()}>New project</button></div>
            {#each latestProjects as project}
              <a class="dashboard-row" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/prj/${encodeURIComponent(project.id)}`} onclick={(event) => { event.preventDefault(); void selectProject(project) }}><span class="dashboard-row-content"><span>{project.name ?? "New Project"}</span><time datetime={project.created_at}>{createdAtLabel(project.created_at)}</time></span></a>
            {:else}<p class="dashboard-empty">No projects yet.</p>{/each}
            <a class="dashboard-view-all" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/prj`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { void selectWorkspaceRoute(projectsPath(activeWorkspace)) } }}>View all projects</a>
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
      {:else if activeSession === null}
        <section class="dashboard-grid">
          <section class="dashboard-widget dashboard-widget-wide">
            <div class="dashboard-widget-heading"><h2>Project Chats</h2><button class="button is-primary is-small" type="button" onclick={() => void createSession(activeProject ?? undefined)}>New chat</button></div>
            {#each projectSessions as session}
              <a class="dashboard-row" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses/${encodeURIComponent(session.id)}`} onclick={(event) => { event.preventDefault(); void selectSession(session) }}><span class="dashboard-row-content"><span>{session.name ?? "New Chat"}</span><span class="dashboard-row-meta"><time datetime={session.created_at}>{createdAtLabel(session.created_at)}</time>{#if session.project !== undefined}<span aria-hidden="true">/</span><span>{session.project.name ?? "New Project"}</span>{/if}</span></span></a>
            {:else}<p class="dashboard-empty">No project chats yet.</p>{/each}
            <a class="dashboard-view-all" href={`/app/wsp/${encodeURIComponent(activeWorkspace?.id ?? "")}/ses`} onclick={(event) => { event.preventDefault(); if (activeWorkspace !== null) { void selectWorkspaceRoute(sessionsPath(activeWorkspace)) } }}>View all chats</a>
          </section>
          <section class="dashboard-widget dashboard-widget-wide project-files-widget">
            <div class="dashboard-widget-heading"><h2>Files</h2><button class="button is-primary is-small" type="button" disabled={uploadingProjectFiles > 0} onclick={() => projectFileInputElement?.click()}>{uploadingProjectFiles > 0 ? "Uploading..." : "Upload files"}</button></div>
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
          {#if messageError !== ""}<p class="help is-danger dashboard-error" aria-live="polite">{messageError}</p>{/if}
        </section>
      {:else}
        <section class="chat-pane">
          <div class="chat-events" aria-live="polite" bind:this={chatEventsElement} onscroll={trackChatScroll}>
            {#if eventStatus === "checking"}
              <p class="chat-status">Loading chat...</p>
            {:else if eventStatus === "unavailable"}
              <p class="chat-status">This chat could not be loaded.</p>
            {:else if events.length === 0}
              <p class="chat-status">Send the first message to begin.</p>
            {:else}
              {#each events as tree (tree.event.ref.id)}
                {#if tree.event.kind === "message.text" && (tree.event.payload.text !== undefined || (tree.event.payload.files !== undefined && tree.event.payload.files.length > 0))}
                  <article class="chat-message message-own">
                    <p class="chat-message-author">{tree.event.author_principal?.name ?? "User"}</p>
                    {#if tree.event.payload.text !== undefined}
                      <button class="chat-message-copy" type="button" aria-label="Copy message Markdown" title="Copy Markdown" onclick={() => void copyMarkdown(tree.event.payload.text)}>
                        <Copy size={16} strokeWidth={2} />
                      </button>
                      <div class="chat-message-text">{@html renderMarkdown(tree.event.payload.text)}</div>
                    {/if}
                    {#if tree.event.payload.files !== undefined && tree.event.payload.files.length > 0}
                      <div class="message-files" aria-label="Attached files">
                        {#each tree.event.payload.files as file (file.id)}
                          <span class="message-file" title={file.fingerprint}>
                            <Paperclip size={14} strokeWidth={2} aria-hidden="true" />
                            <span>{file.name}</span>
                            <small>{file.size} bytes{file.media_type === undefined ? "" : ` · ${file.media_type}`}</small>
                          </span>
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
                                Action: {activity.event.payload.reason ?? `Running ${activity.event.payload.name ?? "tool"}`}
                                {#if toolCallDuration(activity) !== ""}
                                  <span class="tool-call-duration">{toolCallDuration(activity)}</span>
                                {/if}
                              </p>
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
{/if}
