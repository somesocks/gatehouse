<script lang="ts">
  import {
    Dialog as Modal,
    DropdownMenu,
    Menu,
    X,
    PageBody,
    PageHeading,
    RouterLink,
    StatusPage,
  } from "../../ui"
  import { workspaceNavigation } from "../../app/navigation"
  import * as UI from "../../ui"
  import {
    createProjectChat,
    fetchProjectChats,
    type Chat,
    type ChatSearchResponse,
  } from "../../app/chats"
  import {
    fetchProjectFiles,
    finishProjectFileUpload,
    startProjectFileUpload,
    type ProjectFile,
  } from "../../app/project-files"
  import { fetchProjectNotes, type ProjectNote } from "../../app/project-notes"
  import {
    fetchProjectRecordSchemas,
    type ProjectRecordSchema,
  } from "../../app/project-records"
  import { fetchProject, updateProject, type Project } from "../../app/projects"
  import {
    fetchProjectSecrets,
    type ProjectSecret,
  } from "../../app/project-secrets"
  import {
    fetchProjectTasks,
    type ProjectTask,
    type TaskAuthor,
    type TaskStatus,
  } from "../../app/project-tasks"
  import { useRuntime } from "../../app/runtime.svelte"
  import * as SidebarPage from "../../ui/sidebar-page"

  import type { Route } from "../../route"

  type DashboardRoute = Extract<Route, { kind: "project" }>
  type Status = "checking" | "ready" | "unavailable"

  const runtime = useRuntime()
  const { activity, access, auth } = runtime
  let project = $state<Project | null>(null)
  let projectStatus = $state<Status>("checking")
  let chats = $state<Chat[]>([])
  let files = $state<ProjectFile[]>([])
  let filesStatus = $state<Status>("checking")
  let filesError = $state("")
  let uploadingFiles = $state(0)
  let fileInput = $state<HTMLInputElement | undefined>()
  let notes = $state<ProjectNote[]>([])
  let notesStatus = $state<Status>("checking")
  let tasks = $state<ProjectTask[]>([])
  let tasksStatus = $state<Status>("checking")
  let secrets = $state<ProjectSecret[]>([])
  let secretsStatus = $state<Status>("checking")
  let recordSchemas = $state<ProjectRecordSchema[]>([])
  let recordSchemasStatus = $state<Status>("checking")
  let actionError = $state("")
  let creatingChat = $state(false)
  let editOpen = $state(false)
  let editName = $state("")
  let editDescription = $state("")
  let editError = $state("")
  let updatingProject = $state(false)
  let generation = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as DashboardRoute)
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const projectPath = (workspaceID: string, projectID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}/prj/${encodeURIComponent(projectID)}`
  const chatsPath = (workspaceID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}/ses`
  const chatPath = (workspaceID: string, sessionID: string) =>
    `${chatsPath(workspaceID)}/${encodeURIComponent(sessionID)}`
  const notesPath = (workspaceID: string, projectID: string) =>
    `${projectPath(workspaceID, projectID)}/pnt`
  const tasksPath = (workspaceID: string, projectID: string) =>
    `${projectPath(workspaceID, projectID)}/tasks`
  const notePath = (workspaceID: string, projectID: string, noteID: string) =>
    `${notesPath(workspaceID, projectID)}/${encodeURIComponent(noteID)}`
  const taskPath = (workspaceID: string, projectID: string, taskID: string) =>
    `${tasksPath(workspaceID, projectID)}/${encodeURIComponent(taskID)}`
  const secretsPath = (workspaceID: string, projectID: string) =>
    `${projectPath(workspaceID, projectID)}/secrets`
  const secretPath = (
    workspaceID: string,
    projectID: string,
    secretID: string,
  ) => `${secretsPath(workspaceID, projectID)}/${encodeURIComponent(secretID)}`
  const recordsPath = (workspaceID: string, projectID: string) =>
    `${projectPath(workspaceID, projectID)}/records`
  const recordSchemaPath = (
    workspaceID: string,
    projectID: string,
    schemaID: string,
  ) => `${recordsPath(workspaceID, projectID)}/${encodeURIComponent(schemaID)}`
  const dateLabel = (value: string) => {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const part = (number: number) => number.toString().padStart(2, "0")
    return `${date.getFullYear()}-${part(date.getMonth() + 1)}-${part(date.getDate())} ${part(date.getHours())}:${part(date.getMinutes())}`
  }
  const sortByCreated = <T extends { id: string; created_at: string }>(
    loaded: T[],
  ) =>
    [...loaded].sort((left, right) => {
      const difference =
        new Date(right.created_at).getTime() -
        new Date(left.created_at).getTime()
      return Number.isFinite(difference) && difference !== 0
        ? difference
        : right.id.localeCompare(left.id)
    })
  const sortSecrets = (loaded: ProjectSecret[]) =>
    [...loaded].sort((left, right) => {
      const difference =
        new Date(right.updated_at).getTime() -
        new Date(left.updated_at).getTime()
      return Number.isFinite(difference) && difference !== 0
        ? difference
        : right.id.localeCompare(left.id)
    })
  const authorLabel = (author: TaskAuthor) =>
    author.principal?.name ??
    author.principal?.id ??
    author.agent?.label ??
    author.agent?.id ??
    author.gateway ??
    "Unknown"
  const statusLabel = (value: TaskStatus) =>
    ({
      draft: "Draft",
      ready: "Ready",
      in_progress: "In progress",
      done: "Done",
      cancelled: "Cancelled",
    })[value]
  const isCurrent = (value: number, workspaceID: string, projectID: string) =>
    value === generation &&
    currentRoute.workspaceID === workspaceID &&
    currentRoute.projectID === projectID &&
    !abortController?.signal.aborted

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })

  function activate(route: DashboardRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    project = null
    projectStatus = "checking"
    chats = []
    files = []
    filesStatus = "checking"
    filesError = ""
    uploadingFiles = 0
    notes = []
    notesStatus = "checking"
    tasks = []
    tasksStatus = "checking"
    secrets = []
    secretsStatus = "checking"
    recordSchemas = []
    recordSchemasStatus = "checking"
    actionError = ""
    creatingChat = false
    editError = ""
    updatingProject = false
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
    route: DashboardRoute,
    value: number,
    signal: AbortSignal,
  ): Promise<void> {
    const { workspaceID, projectID } = route
    if (
      !access.state.workspaces.some((candidate) => candidate.id === workspaceID)
    ) {
      if (isCurrent(value, workspaceID, projectID))
        runtime.navigate(
          access.state.workspaces.length === 0
            ? "/app/no-access"
            : `/app/wsp/${encodeURIComponent(access.state.workspaces[0].id)}`,
          true,
        )
      return
    }
    const loaded = await loadProject(value, workspaceID, projectID, signal)
    if (!loaded || !isCurrent(value, workspaceID, projectID)) return
    subscribe(route, value)
    await Promise.all([
      loadChats(value, workspaceID, projectID, signal),
      loadFiles(value, workspaceID, projectID, signal),
      loadNotes(value, workspaceID, projectID, signal),
      loadTasks(value, workspaceID, projectID, signal),
      loadSecrets(value, workspaceID, projectID, signal),
      loadRecordSchemas(value, workspaceID, projectID, signal),
    ])
  }

  async function loadProject(
    value: number,
    workspaceID: string,
    projectID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, projectID))
      projectStatus = "checking"
    try {
      const response = await fetchProject(workspaceID, projectID, signal)
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (response.status === 404) {
        runtime.navigate(
          `/app/wsp/${encodeURIComponent(workspaceID)}/prj`,
          true,
        )
        return false
      }
      if (!response.ok) throw new Error("project unavailable")
      const loaded = (await response.json()) as Project
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      project = loaded
      projectStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        projectStatus = "unavailable"
      return false
    }
  }

  function subscribe(route: DashboardRoute, value: number): void {
    const { workspaceID, projectID } = route
    unsubscribe = activity.subscribe(
      [
        {
          name: "project-dashboard",
          topic: `${workspaceID}/${projectID}`,
          events: [
            "project.*",
            "project_file.*",
            "project_note.*",
            "project_task.*",
            "project_secret.*",
            "project_record_schema.*",
            "session.*",
          ],
        },
      ],
      async ({ signal }) => {
        if (!isCurrent(value, workspaceID, projectID) || signal.aborted) return
        const refreshed = await Promise.all([
          loadProject(value, workspaceID, projectID, signal, false),
          loadChats(value, workspaceID, projectID, signal),
          loadFiles(value, workspaceID, projectID, signal, false),
          loadNotes(value, workspaceID, projectID, signal, false),
          loadTasks(value, workspaceID, projectID, signal, false),
          loadSecrets(value, workspaceID, projectID, signal, false),
          loadRecordSchemas(value, workspaceID, projectID, signal, false),
        ])
        if (
          !refreshed.every(Boolean) ||
          signal.aborted ||
          !isCurrent(value, workspaceID, projectID)
        )
          throw new Error("project dashboard refresh failed")
      },
    )
    void activity.poll()
  }

  async function loadChats(
    value: number,
    workspaceID: string,
    projectID: string,
    signal: AbortSignal,
  ): Promise<boolean> {
    try {
      const response = await fetchProjectChats(workspaceID, projectID, signal)
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("chats unavailable")
      const loaded = (await response.json()) as ChatSearchResponse
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      chats = loaded.sessions
      return true
    } catch {
      return false
    }
  }

  async function loadFiles(
    value: number,
    workspaceID: string,
    projectID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, projectID))
      filesStatus = "checking"
    try {
      const response = await fetchProjectFiles(workspaceID, projectID, signal)
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("files unavailable")
      const loaded = (await response.json()) as ProjectFile[]
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      files = sortByCreated(loaded)
      filesStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        filesStatus = "unavailable"
      return false
    }
  }

  async function loadNotes(
    value: number,
    workspaceID: string,
    projectID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, projectID))
      notesStatus = "checking"
    try {
      const response = await fetchProjectNotes(workspaceID, projectID, signal)
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("notes unavailable")
      const loaded = (await response.json()) as ProjectNote[]
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      notes = sortByCreated(loaded)
      notesStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        notesStatus = "unavailable"
      return false
    }
  }

  async function loadTasks(
    value: number,
    workspaceID: string,
    projectID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, projectID))
      tasksStatus = "checking"
    try {
      const response = await fetchProjectTasks(workspaceID, projectID, signal)
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("tasks unavailable")
      const loaded = (await response.json()) as ProjectTask[]
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      tasks = sortByCreated(loaded)
      tasksStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        tasksStatus = "unavailable"
      return false
    }
  }

  async function loadSecrets(
    value: number,
    workspaceID: string,
    projectID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, projectID))
      secretsStatus = "checking"
    try {
      const response = await fetchProjectSecrets(workspaceID, projectID, signal)
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("secrets unavailable")
      const loaded = (await response.json()) as ProjectSecret[]
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      secrets = sortSecrets(loaded)
      secretsStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        secretsStatus = "unavailable"
      return false
    }
  }

  async function loadRecordSchemas(
    value: number,
    workspaceID: string,
    projectID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, projectID))
      recordSchemasStatus = "checking"
    try {
      const response = await fetchProjectRecordSchemas(
        workspaceID,
        projectID,
        signal,
      )
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("record schemas unavailable")
      const loaded = (await response.json()) as ProjectRecordSchema[]
      if (!isCurrent(value, workspaceID, projectID) || signal.aborted)
        return false
      recordSchemas = sortByCreated(loaded)
      recordSchemasStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, projectID) && !signal.aborted)
        recordSchemasStatus = "unavailable"
      return false
    }
  }

  async function createChat(): Promise<void> {
    const route = currentRoute
    const value = generation
    creatingChat = true
    actionError = ""
    try {
      const response = await createProjectChat(
        route.workspaceID,
        route.projectID,
        abortController?.signal,
      )
      if (!isCurrent(value, route.workspaceID, route.projectID)) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("chat unavailable")
      const chat = (await response.json()) as Chat
      if (!isCurrent(value, route.workspaceID, route.projectID)) return
      runtime.navigate(chatPath(route.workspaceID, chat.id))
    } catch {
      if (isCurrent(value, route.workspaceID, route.projectID))
        actionError = "A new chat could not be created. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.projectID))
        creatingChat = false
    }
  }

  function openEdit(): void {
    if (project === null) return
    editName = project.name ?? ""
    editDescription = project.description ?? ""
    editError = ""
    editOpen = true
  }
  function closeEdit(): void {
    if (!updatingProject) editOpen = false
  }
  async function saveProject(): Promise<void> {
    const route = currentRoute
    const value = generation
    updatingProject = true
    editError = ""
    try {
      const response = await updateProject(
        route.workspaceID,
        route.projectID,
        { name: editName, description: editDescription },
        abortController?.signal,
      )
      if (!isCurrent(value, route.workspaceID, route.projectID)) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("project unavailable")
      const updated = (await response.json()) as Project
      if (!isCurrent(value, route.workspaceID, route.projectID)) return
      project = updated
      editOpen = false
    } catch {
      if (isCurrent(value, route.workspaceID, route.projectID))
        editError = "The project could not be updated. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.projectID))
        updatingProject = false
    }
  }

  async function uploadFiles(input: HTMLInputElement): Promise<void> {
    const selected = Array.from(input.files ?? [])
    input.value = ""
    if (selected.length === 0) return
    const route = currentRoute
    const value = generation
    filesError = ""
    uploadingFiles += selected.length
    try {
      const results = await Promise.allSettled(
        selected.map((file) =>
          uploadFile(
            route.workspaceID,
            route.projectID,
            file,
            abortController?.signal,
          ),
        ),
      )
      if (!isCurrent(value, route.workspaceID, route.projectID)) return
      await loadFiles(
        value,
        route.workspaceID,
        route.projectID,
        abortController!.signal,
        false,
      )
      if (
        isCurrent(value, route.workspaceID, route.projectID) &&
        results.some((result) => result.status === "rejected")
      )
        filesError = "Some files could not be uploaded. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.projectID))
        uploadingFiles -= selected.length
    }
  }

  async function uploadFile(
    workspaceID: string,
    projectID: string,
    file: File,
    signal?: AbortSignal,
  ): Promise<void> {
    const started = await startProjectFileUpload(
      workspaceID,
      projectID,
      file,
      signal,
    )
    if (started.status === 401) {
      runtime.requireLogin()
      throw new Error("authentication required")
    }
    if (!started.ok) throw new Error("file start failed")
    const upload = (await started.json()) as {
      file: ProjectFile
      upload_url: string
    }
    if (signal?.aborted) throw new Error("upload cancelled")
    const put = await fetch(upload.upload_url, {
      method: "PUT",
      body: file,
      ...(file.type === "" ? {} : { headers: { "Content-Type": file.type } }),
      signal,
    })
    if (!put.ok) throw new Error("file upload failed")
    const finished = await finishProjectFileUpload(
      workspaceID,
      projectID,
      upload.file.id,
      signal,
    )
    if (finished.status === 401) {
      runtime.requireLogin()
      throw new Error("authentication required")
    }
    if (!finished.ok) throw new Error("file finish failed")
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <StatusPage eyebrow="Gatehouse" busy live="polite">
    {#snippet children()}
      <UI.Text as="p">
        {auth.state.status === "checking"
          ? "Checking your session."
          : "Loading your workspaces."}
      </UI.Text>
    {/snippet}
  </StatusPage>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage
    eyebrow="Gatehouse"
    title="Connection unavailable"
    description="Gatehouse could not load your account."
  >
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => void runtime.refresh()}>Try again</UI.Button
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else if auth.state.status !== "authenticated"}
  <StatusPage eyebrow="Gatehouse" title="Sign in required">
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => runtime.requireLogin()}>Sign in</UI.Button
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <StatusPage
    eyebrow="Gatehouse"
    title="No workspace access"
    description="Ask an administrator to add you to a workspace group."
  >
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
  <SidebarPage.Root
    ><SidebarPage.Sidebar
      ><UI.Navigation
        {...workspaceNavigation(runtime, workspace, "projects")}
      /></SidebarPage.Sidebar
    ><SidebarPage.Page
      ><SidebarPage.Header
        ><SidebarPage.Toggle />
        <UI.Breadcrumb.Root aria-label="Breadcrumb"
          ><UI.Breadcrumb.List
            ><UI.Breadcrumb.Item
              ><RouterLink href={`/app/wsp/${encodeURIComponent(workspace.id)}`}
                >{workspace.name ?? workspace.id}</RouterLink
              ></UI.Breadcrumb.Item
            ><UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
              ><RouterLink
                href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj`}
                >Projects</RouterLink
              ></UI.Breadcrumb.Item
            ><UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
              ><UI.Breadcrumb.Page
                >{project?.name ?? "New Project"}</UI.Breadcrumb.Page
              ></UI.Breadcrumb.Item
            ></UI.Breadcrumb.List
          ></UI.Breadcrumb.Root
        ></SidebarPage.Header
      ><SidebarPage.Body
        ><PageBody fluid>
          {#if projectStatus === "checking"}<UI.Notice>
              <UI.Text as="p">Loading project...</UI.Text>
            </UI.Notice>
          {:else if projectStatus === "unavailable"}<UI.Notice>
              <UI.Text as="p">Project unavailable.</UI.Text>
              <UI.Button
                variant="default"
                type="button"
                onclick={() =>
                  void loadRoute(
                    currentRoute,
                    generation,
                    abortController!.signal,
                  )}>Try again</UI.Button
              >
            </UI.Notice>
          {:else if project !== null}
            {#snippet projectActions()}<DropdownMenu.Root
                ><DropdownMenu.Trigger
                  variant="ghost"
                  size="icon"
                  aria-label="Project actions"
                  title="Project actions"
                  ><Menu
                    size={22}
                    strokeWidth={2}
                    aria-hidden="true"
                  /></DropdownMenu.Trigger
                ><DropdownMenu.Content sideOffset={6} align="end"
                  ><DropdownMenu.Item onSelect={openEdit}
                    >Edit project</DropdownMenu.Item
                  ></DropdownMenu.Content
                ></DropdownMenu.Root
              >{/snippet}
            <UI.Stack as="section">
              <PageHeading as="header" actions={projectActions}>
                <UI.Text as="p" size="eyebrow">Project</UI.Text>
                <UI.Heading level={2}>
                  {project.name ?? "New Project"}
                </UI.Heading>
                <UI.Text as="p" tone="muted">
                  {project.description ?? "No description yet."}
                </UI.Text>
              </PageHeading>
              <UI.Grid columns={2}>
                <UI.Panel>
                  <UI.Row justify="between">
                    <UI.Heading level={2}>Project Chats</UI.Heading>
                    <UI.Button
                      variant="default"
                      size="sm"
                      type="button"
                      disabled={creatingChat}
                      onclick={() => void createChat()}
                      >{creatingChat ? "Creating..." : "New chat"}</UI.Button
                    >
                  </UI.Row>
                  {#each chats as chat (chat.id)}<RouterLink
                      variant="entry"
                      href={chatPath(workspace.id, chat.id)}
                      ><UI.Stack as="span" gap="tight"
                        ><UI.Text truncate>{chat.name ?? "New Chat"}</UI.Text
                        ><UI.Text size="small" truncate
                          ><UI.Text as="time" datetime={chat.created_at}
                            >{dateLabel(chat.created_at)}</UI.Text
                          ></UI.Text
                        ></UI.Stack
                      ></RouterLink
                    >{:else}<UI.Notice>
                      No project chats yet.
                    </UI.Notice>{/each}<RouterLink
                    href={chatsPath(workspace.id)}>View all chats</RouterLink
                  >
                </UI.Panel>
                <UI.Panel>
                  <UI.Row justify="between">
                    <UI.Heading level={2}>Project Files</UI.Heading>
                    <UI.Button
                      variant="default"
                      size="sm"
                      type="button"
                      disabled={uploadingFiles > 0}
                      onclick={() => fileInput?.click()}
                      >{uploadingFiles > 0
                        ? "Uploading..."
                        : "Upload files"}</UI.Button
                    >
                  </UI.Row>
                  <UI.FileInput
                    hidden
                    type="file"
                    autocomplete="off"
                    multiple
                    bind:ref={fileInput}
                    onchange={(event) => void uploadFiles(event.currentTarget)}
                  />{#if filesStatus === "checking"}<UI.Notice>
                      Loading files...
                    </UI.Notice>{:else if filesStatus === "unavailable"}<UI.Notice
                    >
                      Files could not be loaded.
                    </UI.Notice>
                    <UI.Button
                      variant="default"
                      size="sm"
                      type="button"
                      onclick={() =>
                        void loadFiles(
                          generation,
                          currentRoute.workspaceID,
                          currentRoute.projectID,
                          abortController!.signal,
                        )}>Try again</UI.Button
                    >{:else}{#each files.slice(0, 5) as file (file.id)}<RouterLink
                        variant="entry"
                        href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj/${encodeURIComponent(project.id)}/files/${encodeURIComponent(file.id)}`}
                        ><UI.Stack as="span" gap="tight"
                          ><UI.Text truncate>{file.name}</UI.Text><UI.Text
                            size="small"
                            truncate
                            ><UI.Text as="time" datetime={file.created_at}
                              >{dateLabel(file.created_at)}</UI.Text
                            > / {file.size} bytes{#if file.media_type !== undefined}
                              / {file.media_type}{/if}</UI.Text
                          ></UI.Stack
                        ></RouterLink
                      >{:else}<UI.Notice>
                        No files yet.
                      </UI.Notice>{/each}{/if}<RouterLink
                    href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj/${encodeURIComponent(project.id)}/files`}
                    >View all files</RouterLink
                  >{#if filesError !== ""}<UI.Notice
                      variant="destructive"
                      aria-live="polite"
                    >
                      {filesError}
                    </UI.Notice>{/if}
                </UI.Panel>
                <UI.Panel>
                  <UI.Row justify="between">
                    <UI.Heading level={2}>Project Notes</UI.Heading>
                    <UI.Button
                      variant="default"
                      size="sm"
                      type="button"
                      onclick={() =>
                        runtime.navigate(
                          `${notesPath(workspace.id, currentRoute.projectID)}/new`,
                        )}>New note</UI.Button
                    >
                  </UI.Row>
                  {#if notesStatus === "checking"}<UI.Notice>
                      Loading notes...
                    </UI.Notice>{:else if notesStatus === "unavailable"}<UI.Notice
                    >
                      Notes could not be loaded.
                    </UI.Notice>
                    <UI.Button
                      variant="default"
                      size="sm"
                      type="button"
                      onclick={() =>
                        void loadNotes(
                          generation,
                          currentRoute.workspaceID,
                          currentRoute.projectID,
                          abortController!.signal,
                        )}>Try again</UI.Button
                    >{:else}{#each notes as note (note.id)}<RouterLink
                        variant="entry"
                        href={notePath(workspace.id, project.id, note.id)}
                      >
                        <UI.Stack as="span" gap="tight">
                          <UI.Text truncate>{note.title}</UI.Text>
                          {#if note.description !== "" || note.sensitive}
                            <UI.Text size="small" truncate
                              >{note.sensitive
                                ? "Sensitive"
                                : ""}{#if note.sensitive && note.description !== ""}
                                /
                              {/if}{note.description}</UI.Text
                            >
                          {/if}
                        </UI.Stack>
                      </RouterLink>
                    {:else}<UI.Notice>
                        No notes yet.
                      </UI.Notice>{/each}{/if}<RouterLink
                    href={notesPath(workspace.id, currentRoute.projectID)}
                    >View all notes</RouterLink
                  >
                </UI.Panel>
                <UI.Panel>
                  <UI.Row justify="between">
                    <UI.Heading level={2}>Project Tasks</UI.Heading>
                    <UI.Button
                      variant="default"
                      size="sm"
                      type="button"
                      onclick={() =>
                        runtime.navigate(
                          `${tasksPath(workspace.id, currentRoute.projectID)}/new`,
                        )}>New task</UI.Button
                    >
                  </UI.Row>
                  {#if tasksStatus === "checking"}<UI.Notice>
                      Loading tasks...
                    </UI.Notice>{:else if tasksStatus === "unavailable"}<UI.Notice
                    >
                      Tasks could not be loaded.
                    </UI.Notice>
                    <UI.Button
                      variant="default"
                      size="sm"
                      type="button"
                      onclick={() =>
                        void loadTasks(
                          generation,
                          currentRoute.workspaceID,
                          currentRoute.projectID,
                          abortController!.signal,
                        )}>Try again</UI.Button
                    >{:else}{#each tasks.slice(0, 5) as task (task.id)}<RouterLink
                        variant="entry"
                        href={taskPath(workspace.id, project.id, task.id)}
                        ><UI.Stack as="span" gap="tight"
                          ><UI.Text truncate>{task.title}</UI.Text><UI.Text
                            size="small"
                            truncate
                            >{statusLabel(task.status)}{#if task.sensitive}
                              / Sensitive{/if}</UI.Text
                          ></UI.Stack
                        ></RouterLink
                      >{:else}<UI.Notice>
                        No tasks yet.
                      </UI.Notice>{/each}{/if}<RouterLink
                    href={tasksPath(workspace.id, currentRoute.projectID)}
                    >View all tasks</RouterLink
                  >
                </UI.Panel>
                <UI.Panel>
                  <UI.Row justify="between">
                    <UI.Heading level={2}>Project Secrets</UI.Heading>
                    <UI.Button
                      variant="default"
                      size="sm"
                      type="button"
                      onclick={() =>
                        runtime.navigate(
                          `${secretsPath(workspace.id, currentRoute.projectID)}/new`,
                        )}>New secret</UI.Button
                    >
                  </UI.Row>
                  {#if secretsStatus === "checking"}<UI.Notice>
                      Loading secrets...
                    </UI.Notice>{:else if secretsStatus === "unavailable"}<UI.Notice
                    >
                      Secrets could not be loaded.
                    </UI.Notice>
                    <UI.Button
                      variant="default"
                      size="sm"
                      type="button"
                      onclick={() =>
                        void loadSecrets(
                          generation,
                          currentRoute.workspaceID,
                          currentRoute.projectID,
                          abortController!.signal,
                        )}>Try again</UI.Button
                    >{:else}{#each secrets as secret (secret.id)}<RouterLink
                        variant="entry"
                        href={secretPath(workspace.id, project.id, secret.id)}
                        ><UI.Stack as="span" gap="tight"
                          ><UI.Text>{secret.description}</UI.Text><UI.Row
                            as="span"
                            gap="small"
                            ><UI.Text
                              >{secret.author.name ?? secret.author.id}</UI.Text
                            ><UI.Text as="time" datetime={secret.updated_at}
                              >Updated {dateLabel(secret.updated_at)}</UI.Text
                            ></UI.Row
                          ></UI.Stack
                        ></RouterLink
                      >{:else}<UI.Notice>
                        No secrets yet.
                      </UI.Notice>{/each}{/if}<RouterLink
                    href={secretsPath(workspace.id, currentRoute.projectID)}
                    >View all secrets</RouterLink
                  >
                </UI.Panel>
                <UI.Panel>
                  <UI.Row justify="between">
                    <UI.Heading level={2}>Project Records</UI.Heading>
                  </UI.Row>
                  {#if recordSchemasStatus === "checking"}<UI.Notice>
                      Loading record types...
                    </UI.Notice>{:else if recordSchemasStatus === "unavailable"}<UI.Notice
                    >
                      Record types could not be loaded.
                    </UI.Notice>
                    <UI.Button
                      variant="default"
                      size="sm"
                      type="button"
                      onclick={() =>
                        void loadRecordSchemas(
                          generation,
                          currentRoute.workspaceID,
                          currentRoute.projectID,
                          abortController!.signal,
                        )}>Try again</UI.Button
                    >{:else}{#each recordSchemas.slice(0, 5) as schema (schema.id)}<RouterLink
                        variant="entry"
                        href={recordSchemaPath(
                          workspace.id,
                          project.id,
                          schema.id,
                        )}
                        ><UI.Stack as="span" gap="tight"
                          ><UI.Text>{schema.label}</UI.Text
                          >{#if schema.description !== ""}<UI.Text size="small"
                              >{schema.description}</UI.Text
                            >{/if}</UI.Stack
                        ></RouterLink
                      >{:else}<UI.Notice>
                        No record types yet.
                      </UI.Notice>{/each}<RouterLink
                      href={recordsPath(workspace.id, project.id)}
                      >View all records</RouterLink
                    >{/if}
                </UI.Panel>
              </UI.Grid>
              {#if actionError !== ""}<UI.Notice
                  variant="destructive"
                  aria-live="polite"
                >
                  {actionError}
                </UI.Notice>{/if}
            </UI.Stack>
          {/if}
        </PageBody></SidebarPage.Body
      ></SidebarPage.Page
    ></SidebarPage.Root
  >
  <Modal.Root bind:open={editOpen}
    ><Modal.Content
      ><UI.Form
        onsubmit={(event) => {
          event.preventDefault()
          void saveProject()
        }}
      >
        <UI.Row justify="between">
          <Modal.Title>Edit project</Modal.Title><UI.Button
            variant="ghost"
            size="icon"
            type="button"
            aria-label="Close"
            onclick={closeEdit}
            ><X size={18} strokeWidth={2} aria-hidden="true" /></UI.Button
          >
        </UI.Row>
        <UI.Field>
          <UI.Label for="project-edit-name">Name</UI.Label><UI.Input
            id="project-edit-name"
            autocomplete="off"
            maxlength={256}
            bind:value={editName}
          />
        </UI.Field>
        <UI.Field>
          <UI.Label for="project-edit-description">Description</UI.Label
          ><UI.Textarea
            id="project-edit-description"
            autocomplete="off"
            rows={4}
            maxlength={4096}
            bind:value={editDescription}
          ></UI.Textarea>
        </UI.Field>
        {#if editError !== ""}<UI.Notice
            variant="destructive"
            aria-live="polite"
          >
            {editError}
          </UI.Notice>{/if}
        <UI.Row justify="end">
          <UI.Button
            variant="outline"
            type="button"
            disabled={updatingProject}
            onclick={closeEdit}>Cancel</UI.Button
          ><UI.Button variant="default" type="submit" disabled={updatingProject}
            >{updatingProject ? "Saving..." : "Save changes"}</UI.Button
          >
        </UI.Row>
      </UI.Form></Modal.Content
    ></Modal.Root
  >
{/if}
