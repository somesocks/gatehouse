<script lang="ts">
  import { fetchChatSession } from "../../app/chat"
  import { fetchProject, type Project } from "../../app/projects"
  import {
    type ProjectTask,
    type TaskAuthor,
    type TaskStatus,
    createProjectTask,
    fetchProjectTask,
    fetchProjectTasks,
    removeProjectTask,
    updateProjectTask,
  } from "../../app/project-tasks"
  import {
    type SessionTask,
    createSessionTask,
    fetchSessionTask,
    fetchSessionTasks,
    removeSessionTask,
    updateSessionTask,
  } from "../../app/session-tasks"
  import { useRuntime } from "../../app/runtime.svelte"
  import PageBody from "../../components/PageBody.svelte"
  import PageHeading from "../../components/PageHeading.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import SelectControl from "../../components/SelectControl.svelte"
  import SessionNavigation from "../../components/SessionNavigation.svelte"
  import StatusPage from "../../components/StatusPage.svelte"
  import * as SidebarPage from "../../components/sidebar-page"
  import WorkspaceNavigation from "../../components/WorkspaceNavigation.svelte"
  import { renderMarkdown } from "../../markdown"
  import type { Route } from "../../route"

  type TaskRoute = Extract<
    Route,
    {
      kind:
        | "project-tasks"
        | "project-task-new"
        | "project-task"
        | "session-tasks"
        | "session-task-new"
        | "session-task"
    }
  >
  type ProjectTaskRoute = Extract<
    TaskRoute,
    { kind: "project-tasks" | "project-task-new" | "project-task" }
  >
  type Task = ProjectTask | SessionTask
  type Session = {
    id: string
    name?: string
    project?: { id: string; name?: string }
  }
  type Owner = Project | Session
  type Status = "checking" | "ready" | "unavailable"

  const runtime = useRuntime()
  const { access, activity, auth } = runtime
  const statuses: { value: TaskStatus; label: string }[] = [
    { value: "draft", label: "Draft" },
    { value: "ready", label: "Ready" },
    { value: "in_progress", label: "In progress" },
    { value: "done", label: "Done" },
    { value: "cancelled", label: "Cancelled" },
  ]
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
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const isProjectRoute = (route: TaskRoute): route is ProjectTaskRoute =>
    route.kind.startsWith("project-")
  function updateSessionName(name: string): void {
    if (!isProjectRoute(currentRoute) && owner !== null)
      owner = { ...owner, name }
  }
  const resourceID = (route: TaskRoute) =>
    isProjectRoute(route) ? route.projectID : route.sessionID
  const workspacePath = (workspaceID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}`
  const ownerPath = (route: TaskRoute) =>
    isProjectRoute(route)
      ? `${workspacePath(route.workspaceID)}/prj/${encodeURIComponent(route.projectID)}`
      : `${workspacePath(route.workspaceID)}/ses/${encodeURIComponent(route.sessionID)}`
  const listPath = (route: TaskRoute) => `${ownerPath(route)}/tasks`
  const detailPath = (route: TaskRoute, taskID: string) =>
    `${listPath(route)}/${encodeURIComponent(taskID)}`
  const dateLabel = (value: string) => {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const part = (number: number) => number.toString().padStart(2, "0")
    return `${date.getFullYear()}-${part(date.getMonth() + 1)}-${part(date.getDate())} ${part(date.getHours())}:${part(date.getMinutes())}`
  }
  const authorLabel = (author: TaskAuthor) =>
    author.principal?.name ??
    author.principal?.id ??
    author.agent?.label ??
    author.agent?.id ??
    author.gateway ??
    "Unknown"
  const statusLabel = (value: TaskStatus) =>
    statuses.find((candidate) => candidate.value === value)?.label ?? value
  const isCurrent = (value: number, route: TaskRoute, signal: AbortSignal) =>
    value === generation &&
    !signal.aborted &&
    currentRoute.workspaceID === route.workspaceID &&
    resourceID(currentRoute) === resourceID(route) &&
    isProjectRoute(currentRoute) === isProjectRoute(route)

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
    if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "ready"
    )
      void loadRoute(route, value, abortController.signal)
    else if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "checking"
    )
      void runtime.refresh()
    return () => {
      if (value === generation) {
        abortController?.abort()
        unsubscribe?.()
        unsubscribe = undefined
      }
    }
  }

  async function loadRoute(
    route: TaskRoute,
    value: number,
    signal: AbortSignal,
  ): Promise<void> {
    if (
      !access.state.workspaces.some(
        (candidate) => candidate.id === route.workspaceID,
      )
    ) {
      if (isCurrent(value, route, signal))
        runtime.navigate(
          access.state.workspaces.length === 0
            ? "/app/no-access"
            : workspacePath(access.state.workspaces[0].id),
          true,
        )
      return
    }
    if (!(await loadOwner(route, value, signal))) return
    if (!isCurrent(value, route, signal)) return
    subscribe(route, value, signal)
    const listed = await loadTasks(route, value, signal)
    if (listed && route.kind.endsWith("-task"))
      await loadDetail(route.taskID, route, value, signal)
  }

  async function loadOwner(
    route: TaskRoute,
    value: number,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    if (showLoading && isCurrent(value, route, signal)) ownerStatus = "checking"
    try {
      const response = isProjectRoute(route)
        ? await fetchProject(route.workspaceID, route.projectID, signal)
        : await fetchChatSession(route.workspaceID, route.sessionID, signal)
      if (!isCurrent(value, route, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (response.status === 404) {
        runtime.navigate(
          isProjectRoute(route)
            ? `${workspacePath(route.workspaceID)}/prj`
            : `${workspacePath(route.workspaceID)}/ses`,
          true,
        )
        return false
      }
      if (!response.ok) throw new Error("owner unavailable")
      const loaded = (await response.json()) as Owner
      if (!isCurrent(value, route, signal)) return false
      owner = loaded
      ownerStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, route, signal)) ownerStatus = "unavailable"
      return false
    }
  }

  function subscribe(
    route: TaskRoute,
    value: number,
    signal: AbortSignal,
  ): void {
    const project = isProjectRoute(route)
    const event = project ? "project_task.*" : "session_task.*"
    const ownerEvent = project ? "project.*" : "session.*"
    unsubscribe = activity.subscribe(
      [
        {
          name: project ? "project-tasks" : "session-tasks",
          topic: `${route.workspaceID}/${resourceID(route)}`,
          events: [ownerEvent, event],
        },
      ],
      async ({ signal: refreshSignal }) => {
        if (!isCurrent(value, route, signal) || refreshSignal.aborted) return
        const refreshed = await Promise.all([
          loadOwner(route, value, signal, false),
          loadTasks(route, value, signal, false),
          ...(route.kind.endsWith("-task")
            ? [loadDetail(route.taskID, route, value, signal)]
            : []),
        ])
        if (
          !refreshed.every(Boolean) ||
          refreshSignal.aborted ||
          !isCurrent(value, route, signal)
        )
          throw new Error("tasks refresh failed")
      },
    )
    void activity.poll()
  }

  async function loadTasks(
    route: TaskRoute,
    value: number,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    if (showLoading && isCurrent(value, route, signal)) tasksStatus = "checking"
    try {
      const response = isProjectRoute(route)
        ? await fetchProjectTasks(route.workspaceID, route.projectID, signal)
        : await fetchSessionTasks(route.workspaceID, route.sessionID, signal)
      if (!isCurrent(value, route, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("tasks unavailable")
      const loaded = (await response.json()) as Task[]
      if (!isCurrent(value, route, signal)) return false
      tasks = loaded
      tasksStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, route, signal)) tasksStatus = "unavailable"
      return false
    }
  }

  async function loadDetail(
    taskID: string,
    route: TaskRoute,
    value: number,
    signal: AbortSignal,
  ): Promise<boolean> {
    if (isCurrent(value, route, signal)) detailStatus = "checking"
    try {
      const response = isProjectRoute(route)
        ? await fetchProjectTask(
            route.workspaceID,
            route.projectID,
            taskID,
            signal,
          )
        : await fetchSessionTask(
            route.workspaceID,
            route.sessionID,
            taskID,
            signal,
          )
      if (!isCurrent(value, route, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (response.status === 404) {
        runtime.navigate(listPath(route), true)
        return false
      }
      if (!response.ok) throw new Error("task unavailable")
      const loaded = (await response.json()) as Task
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
    if (currentRoute.kind.endsWith("-task-new"))
      runtime.navigate(listPath(currentRoute))
    else editing = false
  }

  async function save(): Promise<void> {
    if (title.trim() === "") {
      error = "Title is required."
      return
    }
    const route = currentRoute
    const value = generation
    const signal = abortController?.signal
    if (signal === undefined) return
    const input = { title, description, sensitive, status }
    saving = true
    error = ""
    try {
      const response = route.kind.endsWith("-task-new")
        ? isProjectRoute(route)
          ? await createProjectTask(
              route.workspaceID,
              route.projectID,
              input,
              signal,
            )
          : await createSessionTask(
              route.workspaceID,
              route.sessionID,
              input,
              signal,
            )
        : active === null
          ? undefined
          : isProjectRoute(route)
            ? await updateProjectTask(
                route.workspaceID,
                route.projectID,
                active.id,
                input,
                signal,
              )
            : await updateSessionTask(
                route.workspaceID,
                route.sessionID,
                active.id,
                input,
                signal,
              )
      if (response === undefined || !isCurrent(value, route, signal)) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("task could not be saved")
      const saved = (await response.json()) as Task
      if (!isCurrent(value, route, signal)) return
      runtime.navigate(detailPath(route, saved.id))
    } catch {
      if (isCurrent(value, route, signal))
        error = "The task could not be saved. Try again."
    } finally {
      if (isCurrent(value, route, signal)) saving = false
    }
  }

  async function remove(): Promise<void> {
    if (
      active === null ||
      deleting ||
      !window.confirm(`Remove ${active.title}?`)
    )
      return
    const route = currentRoute
    const task = active
    const value = generation
    const signal = abortController?.signal
    if (signal === undefined) return
    deleting = true
    error = ""
    try {
      const response = isProjectRoute(route)
        ? await removeProjectTask(
            route.workspaceID,
            route.projectID,
            task.id,
            signal,
          )
        : await removeSessionTask(
            route.workspaceID,
            route.sessionID,
            task.id,
            signal,
          )
      if (!isCurrent(value, route, signal) || active?.id !== task.id) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("task could not be removed")
      tasks = tasks.filter((candidate) => candidate.id !== task.id)
      runtime.navigate(listPath(route))
    } catch {
      if (isCurrent(value, route, signal))
        error = "The task could not be removed. Try again."
    } finally {
      if (isCurrent(value, route, signal)) deleting = false
    }
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <StatusPage busy>
    {#snippet children()}
      <p>Loading your workspaces.</p>
    {/snippet}
  </StatusPage>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage title="Connection unavailable">
    {#snippet children()}<div><button class="primary" type="button" onclick={() => void runtime.refresh()}>Try again</button></div>{/snippet}
  </StatusPage>
{:else if auth.state.status !== "authenticated"}
  <StatusPage title="Sign in required">
    {#snippet children()}<div><button class="primary" type="button" onclick={() => runtime.requireLogin()}>Sign in</button></div>{/snippet}
  </StatusPage>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <StatusPage title="No workspace access">
    {#snippet children()}<div><button class="primary" type="button" onclick={() => runtime.navigate("/app/no-access", true)}>Continue</button></div>{/snippet}
  </StatusPage>
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar
      ><WorkspaceNavigation
        {workspace}
        active={isProjectRoute(currentRoute) ? "projects" : "chats"}
      /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header>
        <SidebarPage.Toggle />
        <nav aria-label="Breadcrumb">
          <ol>
            <li><RouterLink href={workspacePath(workspace.id)}>{workspace.name ?? workspace.id}</RouterLink></li>
            {#if isProjectRoute(currentRoute)}
              <li><RouterLink href={`${workspacePath(workspace.id)}/prj`}>Projects</RouterLink></li>
              <li><RouterLink href={ownerPath(currentRoute)}>{owner?.name ?? "New Project"}</RouterLink></li>
            {:else}
              {#if owner?.project !== undefined}
                <li><RouterLink href={`${workspacePath(workspace.id)}/prj/${encodeURIComponent(owner.project.id)}`}>{owner.project.name ?? "New Project"}</RouterLink></li>
              {/if}
              <li><RouterLink href={ownerPath(currentRoute)}>{owner?.name ?? "New Chat"}</RouterLink></li>
            {/if}
            {#if currentRoute.kind.endsWith("tasks")}
              <li aria-current="page">Tasks</li>
            {:else}
              <li><RouterLink href={listPath(currentRoute)}>Tasks</RouterLink></li>
              <li aria-current="page">{currentRoute.kind.endsWith("new") ? "New Task" : (active?.title ?? "Task")}</li>
            {/if}
          </ol>
        </nav>
        {#if !isProjectRoute(currentRoute) && owner !== null}<SessionNavigation
            workspaceID={workspace.id}
            sessionID={currentRoute.sessionID}
            active="tasks"
            name={owner.name}
            onRenamed={updateSessionName}
          />{/if}</SidebarPage.Header
      >
      <SidebarPage.Body
        ><PageBody fluid>
          {#if ownerStatus === "checking"}<p class="muted">
              Loading {isProjectRoute(currentRoute) ? "project" : "chat"}...
            </p>
          {:else if ownerStatus === "unavailable"}<p class="muted">
              This {isProjectRoute(currentRoute) ? "project" : "chat"} could not
              be loaded.
            </p>
            <button
              class="primary"
              type="button"
              onclick={() => {
                const signal = abortController?.signal
                if (signal !== undefined)
                  void loadRoute(currentRoute, generation, signal)
              }}>Try again</button
            >
          {:else if editing}<form
              class="stack"
              onsubmit={(event) => {
                event.preventDefault()
                void save()
              }}
            >
              <PageHeading>
                <p class="eyebrow">
                  {isProjectRoute(currentRoute) ? "Project" : "Session"} Task
                </p>
                <h2>
                  {currentRoute.kind.endsWith("new") ? "New Task" : "Edit Task"}
                </h2>
              </PageHeading>
              <div class="field">
                <label for="task-title">Title</label>
                <input
                  id="task-title"
                  autocomplete="off"
                  maxlength="256"
                  required
                  bind:value={title}
                />
              </div>
              <div class="field">
                <label for="task-status">Status</label>
                <SelectControl>
                  <select id="task-status" bind:value={status}
                    >{#each statuses as option}<option value={option.value}
                        >{option.label}</option
                      >{/each}</select
                  >
                </SelectControl>
              </div>
              <div class="field">
                <label for="task-description"
                  >Description (optional Markdown)</label
                >
                <textarea
                  id="task-description"
                  autocomplete="off"
                  rows="12"
                  maxlength="4096"
                  bind:value={description}></textarea>
              </div>
              <div class="field">
                <label class="choice"
                  ><input
                    type="checkbox"
                    autocomplete="off"
                    bind:checked={sensitive}
                  /> Sensitive: the description is marked sensitive when agents read
                  it.</label
                >
              </div>
              {#if error !== ""}<p class="field-help" role="alert" aria-live="polite">
                  {error}
                </p>{/if}
              <div class="cluster">
                <button
                  type="button"
                  disabled={saving}
                  onclick={cancelEdit}>Cancel</button
                ><button
                  class="primary"
                  type="submit"
                  disabled={saving}>{saving ? "Saving..." : "Save task"}</button
                >
              </div>
            </form>
          {:else if currentRoute.kind.endsWith("-task") && detailStatus === "checking"}<p
              class="muted"
            >
              Loading task...
            </p>
          {:else if currentRoute.kind.endsWith("-task") && detailStatus === "unavailable"}<p
              class="muted"
            >
              Task unavailable.
            </p>
            <button
              class="primary"
              type="button"
              onclick={() => {
                const signal = abortController?.signal
                if (signal !== undefined && currentRoute.kind.endsWith("-task"))
                  void loadDetail(
                    currentRoute.taskID,
                    currentRoute,
                    generation,
                    signal,
                  )
              }}>Try again</button
            >
          {:else if active !== null}<article class="stack">
              {#snippet taskActions()}<button
                  class="small"
                  type="button"
                  onclick={startEdit}>Edit</button
                ><button
                  class="secondary small"
                  type="button"
                  disabled={deleting}
                  onclick={() => void remove()}
                  >{deleting ? "Removing..." : "Remove"}</button
                >{/snippet}
              <PageHeading as="header" actions={taskActions}>
                <p class="eyebrow">
                  {isProjectRoute(currentRoute) ? "Project" : "Session"} Task
                </p>
                <h2>
                  {active.title}
                  <span class="badge"
                    >{statusLabel(active.status)}</span
                  >{#if active.sensitive}<span class="badge"
                      >Sensitive</span
                    >{/if}
                </h2>
                {#if isProjectRoute(currentRoute)}<small
                    >{dateLabel(active.created_at)}</small
                  >{:else}<small
                    >Created {dateLabel(active.created_at)} by {authorLabel(
                      active.creator,
                    )}. Updated {dateLabel(active.updated_at)} by {authorLabel(
                      active.updater,
                    )}.</small
                  >{/if}
              </PageHeading>
              {#if active.description !== undefined && active.description !== ""}<div
                  class="prose"
                >
                  {@html renderMarkdown(active.description)}
                </div>{:else}<p class="muted">
                  No description.
                </p>{/if}{#if error !== ""}<p
                  class="field-help"
                  role="alert"
                  aria-live="polite"
                >
                  {error}
                </p>{/if}
            </article>
          {:else}{#snippet collectionActions()}<button
                class="primary small"
                type="button"
                onclick={() =>
                  runtime.navigate(`${listPath(currentRoute)}/new`)}
                >New task</button
              >{/snippet}
            <PageHeading actions={collectionActions}>
              <h2>
                {isProjectRoute(currentRoute) ? "Project" : "Session"} Tasks
              </h2>
            </PageHeading>
            <div class="list">
              {#if tasksStatus === "checking"}<p class="muted">
                  Loading tasks...
                </p>{:else if tasksStatus === "unavailable"}<p
                  class="muted"
                >
                  Tasks could not be loaded.
                </p>
                <button
                  class="primary small"
                  type="button"
                  onclick={() => {
                    const signal = abortController?.signal
                    if (signal !== undefined)
                      void loadTasks(currentRoute, generation, signal)
                  }}>Try again</button
                >{:else}{#each tasks as task (task.id)}<RouterLink
                    class="list-item surface stack"
                    href={detailPath(currentRoute, task.id)}
                    ><span class="stack"
                      ><span
                        ><strong>{task.title}</strong>
                        <span class="badge"
                          >{statusLabel(task.status)}</span
                        >{#if task.sensitive}<span class="badge"
                            >Sensitive</span
                          >{/if}</span
                      ><small class="cluster muted"
                        >{#if isProjectRoute(currentRoute)}<time
                            datetime={task.created_at}
                            >{dateLabel(task.created_at)}</time
                          >{:else}<span
                            >Created {dateLabel(task.created_at)} by {authorLabel(
                              task.creator,
                            )}</span
                          ><span
                            >Updated {dateLabel(task.updated_at)} by {authorLabel(
                              task.updater,
                            )}</span
                          >{/if}</small
                      ></span
                    ></RouterLink
                  >{:else}<p class="muted">
                    No tasks yet.
                  </p>{/each}{/if}
            </div>{/if}
        </PageBody></SidebarPage.Body
      >
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
