<script lang="ts">
  import { onMount, tick } from "svelte"
  import { Bot, CircleCheck, CircleX, Copy, Menu, Paperclip, Send, X } from "@lucide/svelte"
  import { renderMarkdown } from "./markdown"

  type Claims = {
    principal: string
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
  }

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
    author_principal?: { id: string }
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
  let agents = $state<WorkspaceAgent[]>([])
  let selectedAgent = $state("")
  let sessions = $state<Session[]>([])
  let activeSession = $state<Session | null>(null)
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
  let pollTimer: ReturnType<typeof setTimeout> | undefined

  onMount(() => {
    currentPath = window.location.pathname
    const handlePopState = () => {
      currentPath = window.location.pathname
      void checkSession()
    }
    window.addEventListener("popstate", handlePopState)
    void checkSession()
    return () => {
      window.removeEventListener("popstate", handlePopState)
      stopPolling()
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
    const match = /^\/app\/w\/([^/]+)(?:\/|$)/.exec(path)
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
    const match = /^\/app\/w\/[^/]+\/s\/([^/]+)(?:\/|$)/.exec(path)
    if (match === null) {
      return null
    }
    try {
      return decodeURIComponent(match[1])
    } catch {
      return null
    }
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
    stopPolling()
    claims = null
    workspaces = []
    activeWorkspace = null
    groups = []
    agents = []
    selectedAgent = ""
    sessions = []
    activeSession = null
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
        sessions = []
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
    stopPolling()
    activeWorkspace = workspace
    groups = []
    agents = []
    selectedAgent = ""
    sessions = []
    activeSession = null
    events = []
    showJumpToLatest = false
    workspaceContentStatus = "checking"
    if (workspaceIDFromPath(currentPath) !== workspace.id) {
      navigate(`/app/w/${encodeURIComponent(workspace.id)}`, replace)
    }
    try {
      const [groupsResponse, sessionsResponse, agentsResponse] = await Promise.all([
        fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/groups`, { credentials: "same-origin" }),
        fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/sessions`, { credentials: "same-origin" }),
        fetch(`/api/v1/workspaces/${encodeURIComponent(workspace.id)}/agents`, { credentials: "same-origin" }),
      ])
      if (groupsResponse.status === 401 || sessionsResponse.status === 401 || agentsResponse.status === 401) {
        signInRequired()
        return
      }
      if (!groupsResponse.ok || !sessionsResponse.ok || !agentsResponse.ok) {
        throw new Error("workspace data could not be loaded")
      }
      groups = (await groupsResponse.json()) as Group[]
      sessions = (await sessionsResponse.json()) as Session[]
      agents = (await agentsResponse.json()) as WorkspaceAgent[]
      workspaceContentStatus = "ready"
      const sessionID = sessionIDFromPath(currentPath)
      const session = sessions.find((candidate) => candidate.id === sessionID)
      if (session !== undefined) {
        await selectSession(session, true)
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
    stopPolling()
    activeSession = session
    events = []
    showJumpToLatest = false
    eventStatus = "checking"
    messageError = ""
    if (sessionIDFromPath(currentPath) !== session.id) {
      navigate(`/app/w/${encodeURIComponent(activeWorkspace.id)}/s/${encodeURIComponent(session.id)}`, replace)
    }
    await loadSessionEvents(session)
  }

  async function loadSessionEvents(session: Session, showLoading = true) {
    if (activeWorkspace === null) {
      return
    }
    if (showLoading) {
      eventStatus = "checking"
    }
    try {
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(activeWorkspace.id)}/sessions/${encodeURIComponent(session.id)}/events?limit=100`, { credentials: "same-origin" })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("session events could not be loaded")
      }
      const loaded = (await response.json()) as SessionEventTree[]
      if (activeSession?.id !== session.id) {
        return
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
      if (awaitingReplyFor.length === 0) {
        stopPolling()
      }
    } catch {
      if (activeSession?.id === session.id) {
        eventStatus = "unavailable"
      }
    }
  }

  function stopPolling() {
    if (pollTimer !== undefined) {
      clearTimeout(pollTimer)
      pollTimer = undefined
    }
    awaitingReplyFor = []
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

  function pollForReply(session: Session) {
    if (pollTimer !== undefined) {
      return
    }
    const poll = async () => {
      if (activeSession?.id !== session.id || awaitingReplyFor.length === 0) {
        return
      }
      await loadSessionEvents(session, false)
      if (awaitingReplyFor.length === 0) {
        return
      }
      pollTimer = setTimeout(poll, 1000)
    }
    pollTimer = setTimeout(poll, 1000)
  }

  async function createSession() {
    if (activeWorkspace === null) {
      return
    }
    mobileMenuOpen = false
    messageError = ""
    try {
      const response = await fetch(`/api/v1/workspaces/${encodeURIComponent(activeWorkspace.id)}/sessions`, {
        method: "POST",
        credentials: "same-origin",
      })
      if (response.status === 401) {
        signInRequired()
        return
      }
      if (!response.ok) {
        throw new Error("session could not be created")
      }
      const session = (await response.json()) as Session
      sessions = [session, ...sessions]
      await selectSession(session)
    } catch {
      messageError = "A new chat could not be created. Try again."
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
      pollForReply(session)
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
            <input class="input" id="identity" name="identity" autocomplete="off" placeholder="root" required bind:value={identity} />
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
      <p class="subtitle is-6">Ask an administrator to add {claims?.principal} to a workspace group.</p>
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
          <div class="sidebar-section-heading">
            <h2>Chats</h2>
            <button class="new-chat" type="button" onclick={() => void createSession()}>New</button>
          </div>
          {#if workspaceContentStatus === "checking"}
            <p class="sidebar-empty">Loading chats...</p>
          {:else if workspaceContentStatus === "unavailable"}
            <p class="sidebar-empty">Chats unavailable.</p>
          {:else if sessions.length === 0}
            <p class="sidebar-empty">No chats yet.</p>
          {:else}
            <ul>
              {#each sessions as session}
                <li><a class:active={activeSession?.id === session.id} href={`/app/w/${encodeURIComponent(activeWorkspace?.id ?? "")}/s/${encodeURIComponent(session.id)}`} onclick={(event) => {
                  event.preventDefault()
                  void selectSession(session)
                }}>{session.id}</a></li>
              {/each}
            </ul>
          {/if}
        </section>

        <section class="sidebar-section">
          <h2>Groups</h2>
          {#if workspaceContentStatus === "checking"}
            <p class="sidebar-empty">Loading groups...</p>
          {:else if workspaceContentStatus === "unavailable"}
            <p class="sidebar-empty">Groups unavailable.</p>
          {:else if groups.length === 0}
            <p class="sidebar-empty">No groups yet.</p>
          {:else}
            <ul>
              {#each groups as group}
                <li>{group.name ?? group.id}</li>
              {/each}
            </ul>
          {/if}
        </section>
      </nav>

      <div class="sidebar-footer">
        <span>{claims?.principal}</span>
        <button class="button is-small is-danger is-light" type="button" onclick={() => void logout()}>Log out</button>
      </div>
    </aside>

      <main class="workspace-main">
        <header class="workspace-header">
          <button class="mobile-menu-trigger" type="button" aria-label="Open navigation menu" aria-expanded={mobileMenuOpen} onclick={() => mobileMenuOpen = true}>
            <Menu size={20} strokeWidth={2} aria-hidden="true" />
          </button>
          <h1 class="workspace-breadcrumb">
            <span>{activeWorkspace?.name ?? activeWorkspace?.id}</span>
            {#if activeSession !== null}
              <span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>
              <span>{activeSession.id}</span>
            {/if}
          </h1>
      </header>
      {#if activeSession === null}
        <section class="workspace-empty">
          <p class="eyebrow">Chats</p>
          <h2 class="title is-3">Start a new conversation.</h2>
          <p class="subtitle is-6">Create a chat to send the first message.</p>
          <button class="button is-primary" type="button" onclick={() => void createSession()}>New chat</button>
          {#if messageError !== ""}
            <p class="help is-danger" aria-live="polite">{messageError}</p>
          {/if}
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
                    <p class="chat-message-author">{tree.event.author_principal?.id ?? "You"}</p>
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
                  {#if activityEvents(tree).length > 0 || awaitingReplyFor.includes(tree.event.ref.id)}
                    <section class="agent-activity-section">
                      <p class="agent-activity-heading">
                        {hasCancellationSuccess(tree) ? "Cancelled" : cancellationRequest(tree) !== undefined ? "Cancellation requested" : finalReplies(tree).length === 0 ? "Agent is working" : "Agent activity"}
                        {#if finalReplies(tree).length > 0 && replyDuration(tree) !== ""}
                          <span class="agent-activity-duration">{replyDuration(tree)}</span>
                        {/if}
                        {#if awaitingReplyFor.includes(tree.event.ref.id) && cancellationRequest(tree) === undefined}
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
                    <Paperclip size={14} strokeWidth={2} aria-hidden="true" />
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
