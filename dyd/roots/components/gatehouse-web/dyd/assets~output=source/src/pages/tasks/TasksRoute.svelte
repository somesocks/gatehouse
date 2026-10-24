<script lang="ts">
  import { untrack } from "svelte"
  import { sessionToolDismissPath } from "../session-chat/session-tools"
  import { createLoadState } from "../../utils/load-state.svelte"
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
  import SessionViewNavigation from "../../session-participants/SessionViewNavigation.svelte"
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

  import { routePath, type Route } from "../../route"

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

  let { embedded = false, route, suspended = false, dialog }: { embedded?: boolean; route?: Route; suspended?: boolean; dialog?: UI.ResourceDialogControls } = $props()
  const runtime = useRuntime()
  const ownerLoad = createLoadState()
  const tasksLoad = createLoadState()
  const detailLoad = createLoadState()
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
  const ownerStatus = $derived(ownerLoad.status)
  let tasks = $state<Task[]>([])
  const tasksStatus = $derived(tasksLoad.status)
  let active = $state<Task | null>(null)
  let editing = $state(false)
  let saving = $state(false)
  let deleting = $state(false)
  let removal = $state<Task | null>(null)
  let collectionOwner = ""
  let title = $state("")
  let description = $state("")
  let sensitive = $state(false)
  let status = $state<TaskStatus>("draft")
  let error = $state("")
  let generation = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived((route ?? runtime.state.route) as TaskRoute)
  const editor = $derived(currentRoute.kind.endsWith("-task-new") || currentRoute.kind.endsWith("-task-edit"))
  const collection = $derived(currentRoute.kind === "session-tasks" || currentRoute.kind === "project-tasks")
  const dialogTitle = $derived(
    currentRoute.kind.endsWith("-task-new") ? "New task"
      : currentRoute.kind.endsWith("-task-edit") ? "Edit task"
      : collection ? "Search tasks" : active?.title ?? "Task",
  )
  const detailStatus = $derived(
    "taskID" in currentRoute ? detailLoad.status : "ready",
  )
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

  const activation = $derived(
    JSON.stringify([
      currentRoute,
      auth.state.status,
      auth.state.claims?.principal.ref.id,
      access.state.workspaceStatus,
      workspace !== null,
    ]),
  )
  $effect(() => {
    activation
    if (suspended) return
    return untrack(() => activate(currentRoute))
  })

  function activate(route: TaskRoute): () => void {
    const key = JSON.stringify([route.workspaceID, isProjectRoute(route), resourceID(route), auth.state.claims?.principal.ref.id])
    if (key !== collectionOwner) {
      collectionOwner = key
      page.reset()
      ownerLoad.reset()
      tasksLoad.reset()
      owner = null
      tasks = []
    } else page.suspend()
    detailLoad.reset()
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    active = null
    editing = route.kind.endsWith("-task-new")
    saving = false
    deleting = false
    removal = null
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
        page.suspend()
        ownerLoad.stop()
        tasksLoad.stop()
        detailLoad.stop()
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
    const request = ownerLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
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
      return request.commit(() => {
        owner = loaded
      })
    } catch {
      if (isCurrent(value, route, signal))
        request.fail(new Error("Project or chat could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  function subscribe(
    route: TaskRoute,
    value: number,
    signal: AbortSignal,
  ): void {
    unsubscribe?.()
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
          loadOwner(
            route,
            value,
            AbortSignal.any([signal, refreshSignal]),
            false,
          ),
          ...(route.kind.endsWith("-tasks")
            ? [
                loadTasks(
                  route,
                  value,
                  AbortSignal.any([signal, refreshSignal]),
                  false,
                ),
              ]
            : []),
          ...(isDetailRoute(route)
            ? [
                loadDetail(
                  route.taskID,
                  route,
                  value,
                  AbortSignal.any([signal, refreshSignal]),
                ),
              ]
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
    const request = tasksLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
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
      return request.commit(() => {
        tasks = loaded
        page.accept(response)
      })
    } catch {
      if (isCurrent(value, route, signal))
        request.fail(new Error("Tasks could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  async function loadDetail(
    taskID: string,
    route: TaskRoute,
    value: number,
    signal: AbortSignal,
  ): Promise<boolean> {
    const request = detailLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
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
      return request.commit(() => {
        active = loaded
        if (route.kind.endsWith("-edit") && !editing) populateEdit()
      })
    } catch {
      if (isCurrent(value, route, signal))
        request.fail(new Error("Task could not be loaded."))
      return false
    } finally {
      request.finish()
    }
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

  async function save(): Promise<void> {
    if (saving || deleting) return
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
      tasks = tasks.map((task) => task.id === saved.id ? saved : task)
      runtime.navigate(detailPath(route, saved.id))
    } catch {
      if (isCurrent(value, route, signal))
        error = "The task could not be saved. Try again."
    } finally {
      if (isCurrent(value, route, signal)) saving = false
    }
  }

  function requestRemove(): void {
    if (active === null || deleting) return
    error = ""
    removal = { ...active }
  }

  async function remove(): Promise<void> {
    if (removal === null || active?.id !== removal.id || deleting) return
    const route = currentRoute
    const task = removal
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

{#snippet searchControls()}
    <UI.SearchToolbar integrated label="Search tasks" placeholder="Search by title…" bind:value={page.state.search} loading={page.state.busy || tasksLoad.loading} onsearch={() => void page.search(reloadTasks)} />
{/snippet}

{#snippet editorActions()}
  <UI.RouterLink variant="outline" disabled={saving}
    href={embedded && "sessionID" in currentRoute ? sessionToolDismissPath(currentRoute)
      : currentRoute.kind.endsWith("-task-new") || active === null ? listPath(currentRoute) : detailPath(currentRoute, active.id)}>Cancel</UI.RouterLink>
  <UI.Button type="submit" form="task-form" disabled={saving || !editing || ownerStatus !== "ready"}>{saving ? "Saving..." : "Save task"}</UI.Button>
{/snippet}

{#snippet taskActions()}
  {#if active}
    <UI.Button variant="outline" disabled={deleting} onclick={requestRemove}>Remove</UI.Button>
    <UI.RouterLink variant={embedded ? "default" : "outline"} href={`${detailPath(currentRoute, active.id)}/edit`}>Edit</UI.RouterLink>
  {/if}
{/snippet}

{#snippet collectionActions()}
  <UI.RouterLink variant="default" href={`${listPath(currentRoute)}/new`}>New task</UI.RouterLink>
{/snippet}

{#snippet dialogActions()}
  {#if editor}{@render editorActions()}
  {:else if collection}{@render collectionActions()}
  {:else}{@render taskActions()}{/if}
{/snippet}

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
      ><UI.RouterLink
        variant="default"
        href={runtime.loginHref} replace>Sign in</UI.RouterLink
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <StatusPage title="No workspace access">
    {#snippet children()}<UI.Group
      ><UI.RouterLink
        variant="default"
        href="/app/no-access" replace
        >Continue</UI.RouterLink
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else}
  <UI.ResourcePage {embedded} dialog={dialog ? {
    ...dialog,
    title: dialogTitle,
    viewKey: routePath(currentRoute),
    pageKey: collection ? JSON.stringify([page.state.query, page.state.cursor]) : undefined,
    compact: collection,
    busy: saving || deleting,
    dismissible: removal === null,
    includeClose: !editor,
    search: collection ? searchControls : undefined,
    footer: dialogActions,
  } : undefined}>
    {#snippet sidebar()}<UI.Navigation
        {...workspaceNavigation(
          runtime,
          workspace,
          isProjectRoute(currentRoute) ? "projects" : "chats",
        )}
      />{/snippet}
    {#snippet header()}
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
        {#if !isProjectRoute(currentRoute) && owner !== null}<SessionViewNavigation
            workspaceID={workspace.id}
            sessionID={"sessionID" in currentRoute ? currentRoute.sessionID : ""}
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
          />{/if}
    {/snippet}
    <PageBody fluid>
          {#if !embedded && (ownerLoad.reloading || tasksLoad.reloading || detailLoad.reloading)}<UI.Text
              role="status">Refreshing tasks…</UI.Text
            >{/if}
          {#if ownerLoad.state.error || tasksLoad.state.error || detailLoad.state.error}<UI.Notice
              variant="destructive"
              role="alert"
              >{ownerLoad.state.error ||
                tasksLoad.state.error ||
                detailLoad.state.error}<UI.Button
                variant="outline"
                onclick={() =>
                  void loadRoute(
                    currentRoute,
                    generation,
                    abortController!.signal,
                  )}>Retry</UI.Button
              ></UI.Notice
            >{/if}
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
          {:else if editing}{#if !embedded}<UI.AdminHeader
              title={currentRoute.kind.endsWith("new")
                ? "New task"
                : `Edit ${active?.title ?? "task"}`}
              description="Update task information, workflow, and sensitivity."
            />{/if}<UI.Form
              id="task-form"
              surface={!embedded}
              compact={embedded}
              onsubmit={(event) => {
                event.preventDefault()
                void save()
              }}
            >
              <UI.FormSection
                dialog={embedded}
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
                dialog={embedded}
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
                dialog={embedded}
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
              {#if !embedded}<UI.FormActions>{@render editorActions()}</UI.FormActions>{/if}
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
          {:else if active !== null}{@const task = active}
            {#if embedded}
              <UI.ResourceArticle>
                {#snippet metadata()}
                  <UI.Badge variant="secondary">{statusLabel(task.status)}</UI.Badge>
                  {#if task.sensitive}<UI.Badge variant="secondary">Sensitive</UI.Badge>{/if}
                {/snippet}
                {#if active.description}
                  <UI.RichText size="normal" html={renderMarkdown(active.description)} />
                {:else}
                  <UI.Text as="p" tone="muted">No description.</UI.Text>
                {/if}
                {#snippet provenance()}
                  <UI.Text as="p" size="small">Updated {dateLabel(task.updated_at)} · {authorLabel(task.updater)}</UI.Text>
                  <UI.Text as="p" size="small">Created {dateLabel(task.created_at)} · {authorLabel(task.creator)}</UI.Text>
                {/snippet}
              </UI.ResourceArticle>
              {#if error}<UI.Notice variant="destructive" role="alert">{error}</UI.Notice>{/if}
            {:else}
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
            {/if}
          {:else}
            {#if !embedded}
              <UI.AdminHeader
                title="Tasks"
                description="Track work and workflow status."
                actions={collectionActions}
              />
            {/if}
            {#if embedded}
              <UI.RecordList
                plain
                icon="task"
                label="Tasks"
                items={tasks.map((task) => ({ ...task, label: task.title }))}
                href={(task) => detailPath(currentRoute, task.id)}
                loading={page.state.busy || tasksLoad.state.pending}
                scrollable={false}
                emptyMessage={tasksStatus === "checking" ? "Loading…" : tasksStatus === "unavailable" ? "Tasks could not be loaded." : "No matching tasks."}
              >
                {#snippet badges(task)}<UI.Badge>{statusLabel(task.status)}</UI.Badge>{#if task.sensitive}<UI.Badge>Sensitive</UI.Badge>{/if}{/snippet}
                {#snippet details(task)}{dateLabel(task.updated_at)} · {authorLabel(task.updater)}{/snippet}
              </UI.RecordList>
            {:else}
            <UI.SearchToolbar
              label="Search tasks"
              placeholder="Title, description, or status"
              bind:value={page.state.search}
              loading={page.state.busy || tasksLoad.state.pending}
              onsearch={() => void page.search(reloadTasks)}
            />
            <UI.RecordTable
              headers={["Task", "Status", "Sensitivity", "Updated"]}
            >
              {#if tasksStatus !== "ready"}<UI.TableEmpty
                  columns={4}
                  message={tasksStatus === "unavailable"
                    ? "Tasks could not be loaded."
                    : tasksStatus === "checking"
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
            {/if}
            {#if !embedded || page.state.history.length > 0 || page.state.next}<UI.PaginationFooter
              showCount={!embedded}
              noun="tasks"
              count={tasks.length}
              previous={page.state.history.length > 0}
              next={!!page.state.next}
              loading={page.state.busy || tasksLoad.state.pending}
              onprevious={() => void page.previous(reloadTasks)}
              onnext={() => void page.next(reloadTasks)}
            />{/if}
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
    </PageBody>
    {#if removal !== null}
      <UI.ResourceRemoval noun="task" scope={isProjectRoute(currentRoute) ? "project" : "chat"}
        name={removal.title} busy={deleting} {error}
        onCancel={() => { removal = null; error = "" }} onConfirm={() => void remove()} />
    {/if}
  </UI.ResourcePage>
{/if}
