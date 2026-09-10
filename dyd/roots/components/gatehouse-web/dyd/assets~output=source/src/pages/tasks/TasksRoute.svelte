<script lang="ts">
  import { Building, Folder, Menu, MessageSquare, SquareCheckBig as ListTodo } from "@lucide/svelte"
  import { signOut } from "../../app/auth"
  import { fetchChatSession } from "../../app/chat"
  import { fetchProject, type Project } from "../../app/projects"
  import { type ProjectTask, type TaskAuthor, type TaskStatus, createProjectTask, fetchProjectTask, fetchProjectTasks, removeProjectTask, updateProjectTask } from "../../app/project-tasks"
  import { type SessionTask, createSessionTask, fetchSessionTask, fetchSessionTasks, removeSessionTask, updateSessionTask } from "../../app/session-tasks"
  import { useRuntime } from "../../app/runtime.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import SessionNavigation from "../../components/SessionNavigation.svelte"
  import WorkspaceFrame from "../../components/WorkspaceFrame.svelte"
  import { renderMarkdown } from "../../markdown"
  import type { Route } from "../../route"

  type TaskRoute = Extract<Route, { kind: "project-tasks" | "project-task-new" | "project-task" | "session-tasks" | "session-task-new" | "session-task" }>
  type ProjectTaskRoute = Extract<TaskRoute, { kind: "project-tasks" | "project-task-new" | "project-task" }>
  type Task = ProjectTask | SessionTask
  type Session = { id: string; name?: string; project?: { id: string; name?: string } }
  type Owner = Project | Session
  type Status = "checking" | "ready" | "unavailable"

  const runtime = useRuntime()
  const { access, activity, auth } = runtime
  const statuses: { value: TaskStatus; label: string }[] = [
    { value: "draft", label: "Draft" }, { value: "ready", label: "Ready" }, { value: "in_progress", label: "In progress" }, { value: "done", label: "Done" }, { value: "cancelled", label: "Cancelled" },
  ]
  let mobileMenuOpen = $state(false)
  let owner = $state<Owner | null>(null)
  let ownerStatus = $state<Status>("checking")
  let tasks = $state<Task[]>([])
  let tasksStatus = $state<Status>("checking")
  let active = $state<Task | null>(null)
  let detailStatus = $state<Status>("ready")
  let editing = $state(false)
  let saving = $state(false)
  let deleting = $state(false)
  let title = $state("")
  let description = $state("")
  let sensitive = $state(false)
  let status = $state<TaskStatus>("draft")
  let error = $state("")
  let generation = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as TaskRoute)
  const workspace = $derived(access.state.workspaces.find((candidate) => candidate.id === currentRoute.workspaceID) ?? null)
  const isProjectRoute = (route: TaskRoute): route is ProjectTaskRoute => route.kind.startsWith("project-")
  const resourceID = (route: TaskRoute) => isProjectRoute(route) ? route.projectID : route.sessionID
  const workspacePath = (workspaceID: string) => `/app/wsp/${encodeURIComponent(workspaceID)}`
  const ownerPath = (route: TaskRoute) => isProjectRoute(route) ? `${workspacePath(route.workspaceID)}/prj/${encodeURIComponent(route.projectID)}` : `${workspacePath(route.workspaceID)}/ses/${encodeURIComponent(route.sessionID)}`
  const listPath = (route: TaskRoute) => `${ownerPath(route)}/tasks`
  const detailPath = (route: TaskRoute, taskID: string) => `${listPath(route)}/${encodeURIComponent(taskID)}`
  const dateLabel = (value: string) => {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const part = (number: number) => number.toString().padStart(2, "0")
    return `${date.getFullYear()}-${part(date.getMonth() + 1)}-${part(date.getDate())} ${part(date.getHours())}:${part(date.getMinutes())}`
  }
  const authorLabel = (author: TaskAuthor) => author.principal?.name ?? author.principal?.id ?? author.agent?.label ?? author.agent?.id ?? author.gateway ?? "Unknown"
  const statusLabel = (value: TaskStatus) => statuses.find((candidate) => candidate.value === value)?.label ?? value
  const isCurrent = (value: number, route: TaskRoute, signal: AbortSignal) => value === generation && !signal.aborted && currentRoute.workspaceID === route.workspaceID && resourceID(currentRoute) === resourceID(route) && isProjectRoute(currentRoute) === isProjectRoute(route)

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })

  function activate(route: TaskRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    mobileMenuOpen = false
    owner = null
    ownerStatus = "checking"
    tasks = []
    tasksStatus = "checking"
    active = null
    detailStatus = route.kind.endsWith("-task") ? "checking" : "ready"
    editing = route.kind.endsWith("-task-new")
    saving = false
    deleting = false
    title = ""
    description = ""
    sensitive = false
    status = "draft"
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

  async function loadRoute(route: TaskRoute, value: number, signal: AbortSignal): Promise<void> {
    if (!access.state.workspaces.some((candidate) => candidate.id === route.workspaceID)) {
      if (isCurrent(value, route, signal)) runtime.navigate(access.state.workspaces.length === 0 ? "/app/no-access" : workspacePath(access.state.workspaces[0].id), true)
      return
    }
    if (!await loadOwner(route, value, signal)) return
    if (!isCurrent(value, route, signal)) return
    subscribe(route, value, signal)
    const listed = await loadTasks(route, value, signal)
    if (listed && route.kind.endsWith("-task")) await loadDetail(route.taskID, route, value, signal)
  }

  async function loadOwner(route: TaskRoute, value: number, signal: AbortSignal, showLoading = true): Promise<boolean> {
    if (showLoading && isCurrent(value, route, signal)) ownerStatus = "checking"
    try {
      const response = isProjectRoute(route) ? await fetchProject(route.workspaceID, route.projectID, signal) : await fetchChatSession(route.workspaceID, route.sessionID, signal)
      if (!isCurrent(value, route, signal)) return false
      if (response.status === 401) { runtime.requireLogin(); return false }
      if (response.status === 404) { runtime.navigate(isProjectRoute(route) ? `${workspacePath(route.workspaceID)}/prj` : `${workspacePath(route.workspaceID)}/ses`, true); return false }
      if (!response.ok) throw new Error("owner unavailable")
      const loaded = await response.json() as Owner
      if (!isCurrent(value, route, signal)) return false
      owner = loaded
      ownerStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, route, signal)) ownerStatus = "unavailable"
      return false
    }
  }

  function subscribe(route: TaskRoute, value: number, signal: AbortSignal): void {
    const project = isProjectRoute(route)
    const event = project ? "project_task.*" : "session_task.*"
    const ownerEvent = project ? "project.*" : "session.*"
    unsubscribe = activity.subscribe([{ name: project ? "project-tasks" : "session-tasks", topic: `${route.workspaceID}/${resourceID(route)}`, events: [ownerEvent, event] }], async ({ signal: refreshSignal }) => {
      if (!isCurrent(value, route, signal) || refreshSignal.aborted) return
      const refreshed = await Promise.all([loadOwner(route, value, signal, false), loadTasks(route, value, signal, false), ...(route.kind.endsWith("-task") ? [loadDetail(route.taskID, route, value, signal)] : [])])
      if (!refreshed.every(Boolean) || refreshSignal.aborted || !isCurrent(value, route, signal)) throw new Error("tasks refresh failed")
    })
    void activity.poll()
  }

  async function loadTasks(route: TaskRoute, value: number, signal: AbortSignal, showLoading = true): Promise<boolean> {
    if (showLoading && isCurrent(value, route, signal)) tasksStatus = "checking"
    try {
      const response = isProjectRoute(route) ? await fetchProjectTasks(route.workspaceID, route.projectID, signal) : await fetchSessionTasks(route.workspaceID, route.sessionID, signal)
      if (!isCurrent(value, route, signal)) return false
      if (response.status === 401) { runtime.requireLogin(); return false }
      if (!response.ok) throw new Error("tasks unavailable")
      const loaded = await response.json() as Task[]
      if (!isCurrent(value, route, signal)) return false
      tasks = loaded
      tasksStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, route, signal)) tasksStatus = "unavailable"
      return false
    }
  }

  async function loadDetail(taskID: string, route: TaskRoute, value: number, signal: AbortSignal): Promise<boolean> {
    if (isCurrent(value, route, signal)) detailStatus = "checking"
    try {
      const response = isProjectRoute(route) ? await fetchProjectTask(route.workspaceID, route.projectID, taskID, signal) : await fetchSessionTask(route.workspaceID, route.sessionID, taskID, signal)
      if (!isCurrent(value, route, signal)) return false
      if (response.status === 401) { runtime.requireLogin(); return false }
      if (response.status === 404) { runtime.navigate(listPath(route), true); return false }
      if (!response.ok) throw new Error("task unavailable")
      const loaded = await response.json() as Task
      if (!isCurrent(value, route, signal)) return false
      active = loaded
      detailStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, route, signal)) detailStatus = "unavailable"
      return false
    }
  }

  function startEdit(): void {
    if (active === null) return
    title = active.title
    description = active.description ?? ""
    sensitive = active.sensitive
    status = active.status
    error = ""
    editing = true
  }

  function cancelEdit(): void {
    if (saving) return
    error = ""
    if (currentRoute.kind.endsWith("-task-new")) runtime.navigate(listPath(currentRoute))
    else editing = false
  }

  async function save(): Promise<void> {
    if (title.trim() === "") { error = "Title is required."; return }
    const route = currentRoute
    const value = generation
    const signal = abortController?.signal
    if (signal === undefined) return
    const input = { title, description, sensitive, status }
    saving = true
    error = ""
    try {
      const response = route.kind.endsWith("-task-new")
        ? isProjectRoute(route) ? await createProjectTask(route.workspaceID, route.projectID, input, signal) : await createSessionTask(route.workspaceID, route.sessionID, input, signal)
        : active === null ? undefined : isProjectRoute(route) ? await updateProjectTask(route.workspaceID, route.projectID, active.id, input, signal) : await updateSessionTask(route.workspaceID, route.sessionID, active.id, input, signal)
      if (response === undefined || !isCurrent(value, route, signal)) return
      if (response.status === 401) { runtime.requireLogin(); return }
      if (!response.ok) throw new Error("task could not be saved")
      const saved = await response.json() as Task
      if (!isCurrent(value, route, signal)) return
				runtime.navigate(detailPath(route, saved.id))
    } catch {
      if (isCurrent(value, route, signal)) error = "The task could not be saved. Try again."
    } finally {
      if (isCurrent(value, route, signal)) saving = false
    }
  }

  async function remove(): Promise<void> {
    if (active === null || deleting || !window.confirm(`Remove ${active.title}?`)) return
    const route = currentRoute
    const task = active
    const value = generation
    const signal = abortController?.signal
    if (signal === undefined) return
    deleting = true
    error = ""
    try {
      const response = isProjectRoute(route) ? await removeProjectTask(route.workspaceID, route.projectID, task.id, signal) : await removeSessionTask(route.workspaceID, route.sessionID, task.id, signal)
      if (!isCurrent(value, route, signal) || active?.id !== task.id) return
      if (response.status === 401) { runtime.requireLogin(); return }
      if (!response.ok) throw new Error("task could not be removed")
      tasks = tasks.filter((candidate) => candidate.id !== task.id)
      runtime.navigate(listPath(route))
    } catch {
      if (isCurrent(value, route, signal)) error = "The task could not be removed. Try again."
    } finally {
      if (isCurrent(value, route, signal)) deleting = false
    }
  }

  async function logout(): Promise<void> {
    try { await signOut() } finally { runtime.requireLogin() }
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <main class="auth-shell" aria-busy="true"><section class="status-card"><div class="loading-mark" aria-hidden="true"></div><p>Loading your workspaces.</p></section></main>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <main class="auth-shell"><section class="status-card"><h1 class="title is-3">Connection unavailable</h1><button class="button is-primary" type="button" onclick={() => void runtime.refresh()}>Try again</button></section></main>
{:else if auth.state.status !== "authenticated"}
  <main class="auth-shell"><section class="status-card"><h1 class="title is-3">Sign in required</h1><button class="button is-primary" type="button" onclick={() => runtime.requireLogin()}>Sign in</button></section></main>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <main class="auth-shell"><section class="status-card"><h1 class="title is-3">No workspace access</h1><button class="button is-primary" type="button" onclick={() => runtime.navigate("/app/no-access", true)}>Continue</button></section></main>
{:else}
  <WorkspaceFrame {mobileMenuOpen} onMenuClose={() => mobileMenuOpen = false}>
    {#snippet sidebar()}<RouterLink class="brand" href="/app/">Gatehouse</RouterLink><div class="workspace-switcher"><label for="workspace">Workspace</label><div class="select is-fullwidth"><select id="workspace" value={workspace.id} onchange={(event) => runtime.navigate(workspacePath(event.currentTarget.value))}>{#each access.state.workspaces as candidate (candidate.id)}<option value={candidate.id}>{candidate.name ?? candidate.id}</option>{/each}</select></div></div><nav class="sidebar-nav" aria-label="Workspace navigation"><section class="sidebar-section"><ul><li><RouterLink class={isProjectRoute(currentRoute) ? undefined : "active"} href={`${workspacePath(workspace.id)}/ses`}>Chats</RouterLink></li><li><RouterLink class={isProjectRoute(currentRoute) ? "active" : undefined} href={`${workspacePath(workspace.id)}/prj`}>Projects</RouterLink></li><li><RouterLink href={`${workspacePath(workspace.id)}/grp`}>Groups</RouterLink></li></ul></section></nav>{#if access.state.systemAccess === "available"}<div class="sidebar-system-link"><RouterLink href="/app/system" target="_blank" rel="noopener">System</RouterLink></div>{/if}<div class="sidebar-footer"><span>{auth.state.claims?.principal.name ?? "User"}</span><button class="button is-small is-danger is-light" type="button" onclick={() => void logout()}>Log out</button></div>{/snippet}
    {#snippet header()}<button class="mobile-menu-trigger" type="button" aria-label="Open navigation menu" aria-expanded={mobileMenuOpen} onclick={() => mobileMenuOpen = true}><Menu size={20} strokeWidth={2} aria-hidden="true" /></button><h1 class="workspace-breadcrumb"><RouterLink class="workspace-breadcrumb-segment" href={workspacePath(workspace.id)}><Building size={16} strokeWidth={2} aria-hidden="true" /><span>{workspace.name ?? workspace.id}</span></RouterLink><span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>{#if isProjectRoute(currentRoute)}<RouterLink href={`${workspacePath(workspace.id)}/prj`}>Projects</RouterLink><span class="workspace-breadcrumb-separator" aria-hidden="true">/</span><RouterLink class="workspace-breadcrumb-segment" href={ownerPath(currentRoute)}><Folder size={16} strokeWidth={2} aria-hidden="true" /><span>{owner?.name ?? "New Project"}</span></RouterLink>{:else}{#if owner?.project !== undefined}<RouterLink class="workspace-breadcrumb-segment" href={`${workspacePath(workspace.id)}/prj/${encodeURIComponent(owner.project.id)}`}><Folder size={16} strokeWidth={2} aria-hidden="true" /><span>{owner.project.name ?? "New Project"}</span></RouterLink><span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>{/if}<RouterLink class="workspace-breadcrumb-segment" href={ownerPath(currentRoute)}><MessageSquare size={16} strokeWidth={2} aria-hidden="true" /><span>{owner?.name ?? "New Chat"}</span></RouterLink>{/if}<span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>{#if currentRoute.kind.endsWith("tasks")}<span>Tasks</span>{:else}<RouterLink href={listPath(currentRoute)}>Tasks</RouterLink><span class="workspace-breadcrumb-separator" aria-hidden="true">/</span><span class="workspace-breadcrumb-segment"><ListTodo size={16} strokeWidth={2} aria-hidden="true" />{currentRoute.kind.endsWith("new") ? "New Task" : active?.title ?? "Task"}</span>{/if}</h1>{#if !isProjectRoute(currentRoute) && owner !== null}<SessionNavigation workspaceID={workspace.id} sessionID={currentRoute.sessionID} active="tasks" />{/if}{/snippet}
    <section class="task-page">
      {#if ownerStatus === "checking"}<p class="dashboard-empty">Loading {isProjectRoute(currentRoute) ? "project" : "chat"}...</p>
      {:else if ownerStatus === "unavailable"}<p class="dashboard-empty">This {isProjectRoute(currentRoute) ? "project" : "chat"} could not be loaded.</p><button class="button is-primary" type="button" onclick={() => { const signal = abortController?.signal; if (signal !== undefined) void loadRoute(currentRoute, generation, signal) }}>Try again</button>
      {:else if editing}<form class="task-editor" onsubmit={(event) => { event.preventDefault(); void save() }}><div class="project-note-page-heading"><div><p class="eyebrow">{isProjectRoute(currentRoute) ? "Project" : "Session"} Task</p><h2>{currentRoute.kind.endsWith("new") ? "New Task" : "Edit Task"}</h2></div></div><div class="field"><label class="label" for="task-title">Title</label><div class="control"><input class="input" id="task-title" autocomplete="off" maxlength="256" required bind:value={title} /></div></div><div class="field"><label class="label" for="task-status">Status</label><div class="control select"><select id="task-status" bind:value={status}>{#each statuses as option}<option value={option.value}>{option.label}</option>{/each}</select></div></div><div class="field"><label class="label" for="task-description">Description (optional Markdown)</label><div class="control"><textarea class="textarea task-description-input" id="task-description" autocomplete="off" rows="12" maxlength="4096" bind:value={description}></textarea></div></div><div class="field"><label class="checkbox"><input type="checkbox" autocomplete="off" bind:checked={sensitive} /> Sensitive: the description is marked sensitive when agents read it.</label></div>{#if error !== ""}<p class="help is-danger" aria-live="polite">{error}</p>{/if}<div class="project-note-actions"><button class="button" type="button" disabled={saving} onclick={cancelEdit}>Cancel</button><button class="button is-primary" type="submit" disabled={saving}>{saving ? "Saving..." : "Save task"}</button></div></form>
      {:else if currentRoute.kind.endsWith("-task") && detailStatus === "checking"}<p class="dashboard-empty">Loading task...</p>
      {:else if currentRoute.kind.endsWith("-task") && detailStatus === "unavailable"}<p class="dashboard-empty">Task unavailable.</p><button class="button is-primary" type="button" onclick={() => { const signal = abortController?.signal; if (signal !== undefined && currentRoute.kind.endsWith("-task")) void loadDetail(currentRoute.taskID, currentRoute, generation, signal) }}>Try again</button>
      {:else if active !== null}<article class="task-view"><header class="project-note-page-heading"><div><p class="eyebrow">{isProjectRoute(currentRoute) ? "Project" : "Session"} Task</p><h2>{active.title} <span class="task-status task-status-{active.status}">{statusLabel(active.status)}</span>{#if active.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</h2>{#if isProjectRoute(currentRoute)}<small>{dateLabel(active.created_at)}</small>{:else}<small>Created {dateLabel(active.created_at)} by {authorLabel(active.creator)}. Updated {dateLabel(active.updated_at)} by {authorLabel(active.updater)}.</small>{/if}</div><div class="project-note-actions"><button class="button is-small" type="button" onclick={startEdit}>Edit</button><button class="button is-small is-danger is-light" type="button" disabled={deleting} onclick={() => void remove()}>{deleting ? "Removing..." : "Remove"}</button></div></header>{#if active.description !== undefined && active.description !== ""}<div class="markdown-content task-markdown">{@html renderMarkdown(active.description)}</div>{:else}<p class="dashboard-empty">No description.</p>{/if}{#if error !== ""}<p class="help is-danger" aria-live="polite">{error}</p>{/if}</article>
      {:else}<div class="collection-heading"><h2>{isProjectRoute(currentRoute) ? "Project" : "Session"} Tasks</h2><button class="button is-primary is-small" type="button" onclick={() => runtime.navigate(`${listPath(currentRoute)}/new`)}>New task</button></div><div class="collection-list">{#if tasksStatus === "checking"}<p class="dashboard-empty">Loading tasks...</p>{:else if tasksStatus === "unavailable"}<p class="dashboard-empty">Tasks could not be loaded.</p><button class="button is-primary is-small" type="button" onclick={() => { const signal = abortController?.signal; if (signal !== undefined) void loadTasks(currentRoute, generation, signal) }}>Try again</button>{:else}{#each tasks as task (task.id)}<RouterLink class="dashboard-row task-row" href={detailPath(currentRoute, task.id)}><span class="dashboard-row-content"><span><strong>{task.title}</strong> <span class="task-status task-status-{task.status}">{statusLabel(task.status)}</span>{#if task.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span><span class="dashboard-row-meta">{#if isProjectRoute(currentRoute)}<time datetime={task.created_at}>{dateLabel(task.created_at)}</time>{:else}<span>Created {dateLabel(task.created_at)} by {authorLabel(task.creator)}</span><span>Updated {dateLabel(task.updated_at)} by {authorLabel(task.updater)}</span>{/if}</span></span></RouterLink>{:else}<p class="dashboard-empty">No tasks yet.</p>{/each}{/if}</div>{/if}
    </section>
  </WorkspaceFrame>
{/if}
