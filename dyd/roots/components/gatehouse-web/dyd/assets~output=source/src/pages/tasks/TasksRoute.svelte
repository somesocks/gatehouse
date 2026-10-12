<script lang="ts">
  import {
    PageBody,
    PageHeading,
    RouterLink,
    StatusPage,
    renderMarkdown,
  } from "../../ui"
  import {
    workspaceNavigation,
    sessionViews,
    renameSession,
  } from "../../app/navigation"
  import * as UI from "../../ui"
  import { userAuthorLabel } from "../../app/user-labels"
  import { createCollectionPage } from "../../utils/collection-page.svelte"
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

  import * as SidebarPage from "../../ui/sidebar-page"

  import type { Route } from "../../route"

  type TaskRoute = Extract<
    Route,
    {
      kind:
        | "project-tasks"
        | "project-task-new"
        | "project-task"
        | "project-task-edit"
        | "session-tasks"
        | "session-task-new"
        | "session-task"
        | "session-task-edit"
    }
  >
  type ProjectTaskRoute = Extract<
    TaskRoute,
    {
      kind:
        | "project-tasks"
        | "project-task-new"
        | "project-task"
        | "project-task-edit"
    }
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
  const page = createCollectionPage()
  const reloadTasks = () =>
    loadTasks(currentRoute, generation, abortController!.signal)
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
  const isDetailRoute = (
    route: TaskRoute,
  ): route is Extract<TaskRoute, { taskID: string }> => "taskID" in route
  const ownerProject = $derived(
    owner !== null && "project" in owner ? owner.project : undefined,
  )
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
  const authorLabel = userAuthorLabel
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
    page.reset()
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
    detailStatus = isDetailRoute(route) ? "checking" : "ready"
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
    if (route.kind.endsWith("-tasks")) await loadTasks(route, value, signal)
    if (isDetailRoute(route))
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
          ...(route.kind.endsWith("-tasks")
            ? [loadTasks(route, value, signal, false)]
            : []),
          ...(isDetailRoute(route)
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
      const requested = page.key()
      const response = isProjectRoute(route)
        ? await fetchProjectTasks(
            route.workspaceID,
            route.projectID,
            signal,
            page.params(),
          )
        : await fetchSessionTasks(
            route.workspaceID,
            route.sessionID,
            signal,
            page.params(),
          )
      if (!isCurrent(value, route, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("tasks unavailable")
      const loaded = (await response.json()) as Task[]
      if (!isCurrent(value, route, signal)) return false
      if (requested !== page.key()) return false
      tasks = loaded
      page.accept(response)
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
      if (route.kind.endsWith("-edit") && !editing) populateEdit()
      detailStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, route, signal)) detailStatus = "unavailable"
      return false
    }
  }

  function startEdit(): void {
    if (active) runtime.navigate(`${detailPath(currentRoute, active.id)}/edit`)
  }

  function populateEdit(): void {
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
    else if (active) runtime.navigate(detailPath(currentRoute, active.id))
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
      <UI.Text as="p">Loading your workspaces.</UI.Text>
    {/snippet}
  </StatusPage>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage title="Connection unavailable">
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => void runtime.refresh()}>Try again</UI.Button
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else if auth.state.status !== "authenticated"}
  <StatusPage title="Sign in required">
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => runtime.requireLogin()}>Sign in</UI.Button
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <StatusPage title="No workspace access">
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => runtime.navigate("/app/no-access", true)}
          >Continue</UI.Button
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar
      ><UI.Navigation
        {...workspaceNavigation(
          runtime,
          workspace,
          isProjectRoute(currentRoute) ? "projects" : "chats",
        )}
      /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header>
        <SidebarPage.Toggle />
        <UI.Breadcrumb.Root aria-label="Breadcrumb">
          <UI.Breadcrumb.List>
            <UI.Breadcrumb.Item
              ><RouterLink href={workspacePath(workspace.id)}
                >{workspace.name ?? "Workspace"}</RouterLink
              ></UI.Breadcrumb.Item
            >
            {#if isProjectRoute(currentRoute)}
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                ><RouterLink href={`${workspacePath(workspace.id)}/prj`}
                  >Projects</RouterLink
                ></UI.Breadcrumb.Item
              >
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                ><RouterLink href={ownerPath(currentRoute)}
                  >{owner?.name ?? "New Project"}</RouterLink
                ></UI.Breadcrumb.Item
              >
            {:else}
              {#if ownerProject !== undefined}
                <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                  ><RouterLink
                    href={`${workspacePath(workspace.id)}/prj/${encodeURIComponent(ownerProject.id)}`}
                    >{ownerProject.name ?? "New Project"}</RouterLink
                  ></UI.Breadcrumb.Item
                >
              {/if}
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                ><RouterLink href={ownerPath(currentRoute)}
                  >{owner?.name ?? "New Chat"}</RouterLink
                ></UI.Breadcrumb.Item
              >
            {/if}
            {#if currentRoute.kind.endsWith("tasks")}
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
                ><UI.Breadcrumb.Page>Tasks</UI.Breadcrumb.Page
                ></UI.Breadcrumb.Item
              >
            {:else}
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                ><RouterLink href={listPath(currentRoute)}>Tasks</RouterLink
                ></UI.Breadcrumb.Item
              >
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
                ><UI.Breadcrumb.Page
                  >{currentRoute.kind.endsWith("new")
                    ? "New Task"
                    : (active?.title ?? "Task")}</UI.Breadcrumb.Page
                ></UI.Breadcrumb.Item
              >
            {/if}
          </UI.Breadcrumb.List>
        </UI.Breadcrumb.Root>
        {#if !isProjectRoute(currentRoute) && owner !== null}<UI.ViewNavigation
            items={sessionViews(
              workspace.id,
              "sessionID" in currentRoute ? currentRoute.sessionID : "",
              "tasks",
            )}
            name={owner.name}
            onRename={(name) =>
              renameSession(
                runtime,
                workspace.id,
                "sessionID" in currentRoute ? currentRoute.sessionID : "",
                name,
                updateSessionName,
              )}
          />{/if}</SidebarPage.Header
      >
      <SidebarPage.Body
        ><PageBody fluid>
          {#if ownerStatus === "checking"}<UI.Text as="p" tone="muted">
              Loading {isProjectRoute(currentRoute) ? "project" : "chat"}...
            </UI.Text>
          {:else if ownerStatus === "unavailable"}<UI.Text as="p" tone="muted">
              This {isProjectRoute(currentRoute) ? "project" : "chat"} could not
              be loaded.
            </UI.Text>
            <UI.Button
              variant="default"
              type="button"
              onclick={() => {
                const signal = abortController?.signal
                if (signal !== undefined)
                  void loadRoute(currentRoute, generation, signal)
              }}>Try again</UI.Button
            >
          {:else if editing}<UI.AdminHeader
              title={currentRoute.kind.endsWith("new")
                ? "New task"
                : `Edit ${active?.title ?? "task"}`}
              description="Update task information, workflow, and sensitivity."
            /><UI.Form
              surface
              onsubmit={(event) => {
                event.preventDefault()
                void save()
              }}
            >
              <UI.FormSection
                title="Task information"
                description="Name the task and choose its workflow status."
                ><UI.Field>
                  <UI.Label for="task-title">Title</UI.Label>
                  <UI.Input
                    id="task-title"
                    autocomplete="off"
                    maxlength={256}
                    required
                    bind:value={title}
                  />
                </UI.Field>
                <UI.Field>
                  <UI.Label for="task-status">Status</UI.Label>

                  <UI.Select id="task-status" bind:value={status}
                    >{#each statuses as option}<UI.Option value={option.value}
                        >{option.label}</UI.Option
                      >{/each}</UI.Select
                  >
                </UI.Field>
              </UI.FormSection><UI.FormSection
                title="Description"
                description="Describe the work using Markdown."
                ><UI.Field>
                  <UI.Label for="task-description"
                    >Description (optional Markdown)</UI.Label
                  >
                  <UI.Textarea
                    id="task-description"
                    autocomplete="off"
                    rows={12}
                    maxlength={4096}
                    bind:value={description}
                  ></UI.Textarea>
                </UI.Field>
              </UI.FormSection><UI.FormSection
                title="Sensitivity"
                description="Control how agents handle the description."
                ><UI.Field>
                  <UI.Label layout="choice"
                    ><UI.Checkbox bind:checked={sensitive} /> Sensitive: the description
                    is marked sensitive when agents read it.</UI.Label
                  >
                </UI.Field>
              </UI.FormSection>{#if error !== ""}<UI.Text
                  as="p"
                  size="small"
                  role="alert"
                  aria-live="polite"
                >
                  {error}
                </UI.Text>{/if}
              <UI.FormActions>
                <UI.Button
                  variant="outline"
                  type="button"
                  disabled={saving}
                  onclick={cancelEdit}>Cancel</UI.Button
                ><UI.Button variant="default" type="submit" disabled={saving}
                  >{saving ? "Saving..." : "Save task"}</UI.Button
                >
              </UI.FormActions>
            </UI.Form>
          {:else if isDetailRoute(currentRoute) && detailStatus === "checking"}<UI.Text
              as="p"
              tone="muted"
            >
              Loading task...
            </UI.Text>
          {:else if isDetailRoute(currentRoute) && detailStatus === "unavailable"}<UI.Text
              as="p"
              tone="muted"
            >
              Task unavailable.
            </UI.Text>
            <UI.Button
              variant="default"
              type="button"
              onclick={() => {
                const signal = abortController?.signal
                if (signal !== undefined && isDetailRoute(currentRoute))
                  void loadDetail(
                    currentRoute.taskID,
                    currentRoute,
                    generation,
                    signal,
                  )
              }}>Try again</UI.Button
            >
          {:else if active !== null}{#snippet taskActions()}<UI.Button
                variant="outline"
                size="sm"
                type="button"
                onclick={startEdit}>Edit</UI.Button
              ><UI.Button
                variant="outline"
                size="sm"
                type="button"
                disabled={deleting}
                onclick={() => void remove()}
                >{deleting ? "Removing..." : "Remove"}</UI.Button
              >{/snippet}
            <UI.Stack as="article">
              <UI.AdminHeader
                kind={isProjectRoute(currentRoute)
                  ? "Project task"
                  : "Chat task"}
                title={active.title}
                actions={taskActions}
              />
              <UI.DetailSection title="Task information">
                <UI.Property label="Status"
                  ><UI.Badge>{statusLabel(active.status)}</UI.Badge
                  ></UI.Property
                >
                <UI.Property label="Sensitivity"
                  >{active.sensitive ? "Sensitive" : "Standard"}</UI.Property
                >
                <UI.Property label="Created"
                  >{dateLabel(active.created_at)} · {authorLabel(
                    active.creator,
                  )}</UI.Property
                >
                <UI.Property label="Updated"
                  >{dateLabel(active.updated_at)} · {authorLabel(
                    active.updater,
                  )}</UI.Property
                >
              </UI.DetailSection>
              {#if active.description !== undefined && active.description !== ""}<UI.RichText
                  html={renderMarkdown(active.description)}
                ></UI.RichText>{:else}<UI.Text as="p" tone="muted">
                  No description.
                </UI.Text>{/if}{#if error !== ""}<UI.Text
                  as="p"
                  size="small"
                  role="alert"
                  aria-live="polite"
                >
                  {error}
                </UI.Text>{/if}
            </UI.Stack>
          {:else}{#snippet collectionActions()}<UI.Button
                variant="default"
                size="sm"
                type="button"
                onclick={() =>
                  runtime.navigate(`${listPath(currentRoute)}/new`)}
                >New task</UI.Button
              >{/snippet}
            <UI.AdminHeader
              title="Tasks"
              description="Track work and workflow status."
              actions={collectionActions}
            />
            <UI.SearchToolbar
              label="Search tasks"
              placeholder="Title, description, or status"
              bind:value={page.state.search}
              loading={page.state.busy || tasksStatus === "checking"}
              onsearch={() => void page.search(reloadTasks)}
            />
            <UI.RecordTable
              headers={["Task", "Status", "Sensitivity", "Updated"]}
            >
              {#if tasksStatus !== "ready"}<UI.TableEmpty
                  columns={4}
                  message={tasksStatus === "checking"
                    ? "Loading tasks…"
                    : "Tasks could not be loaded."}
                />
              {:else}{#each tasks as task (task.id)}<UI.RecordRow>
                    <UI.RecordCell primary
                      ><RouterLink href={detailPath(currentRoute, task.id)}
                        >{task.title}</RouterLink
                      ></UI.RecordCell
                    >
                    <UI.RecordCell
                      ><UI.Badge>{statusLabel(task.status)}</UI.Badge
                      ></UI.RecordCell
                    >
                    <UI.RecordCell
                      >{task.sensitive
                        ? "Sensitive"
                        : "Standard"}</UI.RecordCell
                    >
                    <UI.RecordCell>{dateLabel(task.updated_at)}</UI.RecordCell>
                  </UI.RecordRow>{:else}<UI.TableEmpty
                    columns={4}
                    message="No tasks yet."
                  />{/each}{/if}
            </UI.RecordTable>
            <UI.PaginationFooter
              noun="tasks"
              count={tasks.length}
              previous={page.state.history.length > 0}
              next={!!page.state.next}
              loading={page.state.busy || tasksStatus === "checking"}
              onprevious={() => void page.previous(reloadTasks)}
              onnext={() => void page.next(reloadTasks)}
            />
            {#if tasksStatus === "unavailable"}<UI.Button
                variant="outline"
                onclick={() =>
                  void loadTasks(
                    currentRoute,
                    generation,
                    abortController!.signal,
                  )}>Try again</UI.Button
              >{/if}
          {/if}
        </PageBody></SidebarPage.Body
      >
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
