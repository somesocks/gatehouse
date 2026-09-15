<script lang="ts">
  import { Button, Dialog, DropdownMenu } from "bits-ui"
  import { Menu, X } from "@lucide/svelte"
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
  import PageBody from "../../components/PageBody.svelte"
  import PageHeading from "../../components/PageHeading.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import * as SidebarPage from "../../components/sidebar-page"
  import WorkspaceNavigation from "../../components/WorkspaceNavigation.svelte"
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
  <main class="status-page" aria-busy="true" aria-live="polite">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <div class="loading-mark" aria-hidden="true"></div>
      <p>
        {auth.state.status === "checking"
          ? "Checking your session."
          : "Loading your workspaces."}
      </p>
    </section>
  </main>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <main class="status-page">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">Connection unavailable</h1>
      <p class="subtitle is-6">Gatehouse could not load your account.</p>
      <button
        class="button is-primary"
        type="button"
        onclick={() => void runtime.refresh()}>Try again</button
      >
    </section>
  </main>
{:else if auth.state.status !== "authenticated"}
  <main class="status-page">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">Sign in required</h1>
      <button
        class="button is-primary"
        type="button"
        onclick={() => runtime.requireLogin()}>Sign in</button
      >
    </section>
  </main>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <main class="status-page">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">No workspace access</h1>
      <p class="subtitle is-6">
        Ask an administrator to add you to a workspace group.
      </p>
      <button
        class="button is-primary"
        type="button"
        onclick={() => runtime.navigate("/app/no-access", true)}
        >Continue</button
      >
    </section>
  </main>
{:else}
  <SidebarPage.Root
    ><SidebarPage.Sidebar
      ><WorkspaceNavigation
        {workspace}
        active="projects"
      /></SidebarPage.Sidebar
    ><SidebarPage.Page
      ><SidebarPage.Header
        ><SidebarPage.Toggle
          ><button
            class="mobile-menu-trigger"
            type="button"
            aria-label="Open navigation menu"
            ><Menu size={20} strokeWidth={2} aria-hidden="true" /></button
          ></SidebarPage.Toggle
        >
        <h1 class="brand-workspace-breadcrumb">
          <RouterLink
            class="brand-workspace-breadcrumb-segment"
            href={`/app/wsp/${encodeURIComponent(workspace.id)}`}
            ><span>{workspace.name ?? workspace.id}</span></RouterLink
          ><span class="brand-workspace-breadcrumb-separator" aria-hidden="true"
            >/</span
          ><RouterLink href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj`}
            >Projects</RouterLink
          ><span class="brand-workspace-breadcrumb-separator" aria-hidden="true"
            >/</span
          ><span class="brand-workspace-breadcrumb-segment"
            >{project?.name ?? "New Project"}</span
          >
        </h1></SidebarPage.Header
      ><SidebarPage.Body><PageBody>
        {#if projectStatus === "checking"}<section
            class="brand-dashboard brand-dashboard-status"
          >
            <p>Loading project...</p>
          </section>
        {:else if projectStatus === "unavailable"}<section
            class="brand-dashboard brand-dashboard-status"
          >
            <p>Project unavailable.</p>
            <Button.Root
              class="brand-button brand-button--primary"
              type="button"
              onclick={() =>
                void loadRoute(
                  currentRoute,
                  generation,
                  abortController!.signal,
                )}>Try again</Button.Root
            >
          </section>
        {:else if project !== null}
          <section>
            {#snippet projectActions()}<DropdownMenu.Root
                ><DropdownMenu.Trigger
                  class="brand-icon-button"
                  aria-label="Project actions"
                  title="Project actions"
                  ><Menu
                    size={22}
                    strokeWidth={2}
                    aria-hidden="true"
                  /></DropdownMenu.Trigger
                ><DropdownMenu.Portal
                  ><DropdownMenu.Content
                    class="brand-menu-content"
                    sideOffset={6}
                    align="end"
                    ><DropdownMenu.Item
                      class="brand-menu-item"
                      onSelect={openEdit}>Edit project</DropdownMenu.Item
                    ></DropdownMenu.Content
                  ></DropdownMenu.Portal
                ></DropdownMenu.Root
              >{/snippet}
            <PageHeading as="header" actions={projectActions}>
              <p class="eyebrow">PROJECT</p>
              <h2>
                {project.name ?? "New Project"}
              </h2>
              <p class="subtitle is-6">
                {project.description ?? "No description yet."}
              </p>
            </PageHeading>
            <section class="brand-dashboard-grid">
              <section class="brand-dashboard-card">
                <div class="brand-card-heading">
                  <h2 class="brand-card-title">Project Chats</h2>
                  <Button.Root
                    class="brand-button brand-button--primary brand-button--compact"
                    type="button"
                    disabled={creatingChat}
                    onclick={() => void createChat()}
                    >{creatingChat ? "Creating..." : "New chat"}</Button.Root
                  >
                </div>
                {#each chats as chat (chat.id)}<RouterLink
                    class="brand-dashboard-row"
                    href={chatPath(workspace.id, chat.id)}
                    ><span class="brand-row-content"
                      ><span>{chat.name ?? "New Chat"}</span><span
                        class="brand-row-meta"
                        ><time datetime={chat.created_at}
                          >{dateLabel(chat.created_at)}</time
                        >{#if chat.project !== undefined}<span
                            aria-hidden="true">/</span
                          ><span>{chat.project.name ?? "New Project"}</span
                          >{/if}</span
                      ></span
                    ></RouterLink
                  >{:else}<p class="brand-empty">
                    No project chats yet.
                  </p>{/each}<RouterLink
                  class="brand-view-all"
                  href={chatsPath(workspace.id)}>View all chats</RouterLink
                >
              </section>
              <section class="brand-dashboard-card">
                <div class="brand-card-heading">
                  <h2 class="brand-card-title">Project Files</h2>
                  <Button.Root
                    class="brand-button brand-button--primary brand-button--compact"
                    type="button"
                    disabled={uploadingFiles > 0}
                    onclick={() => fileInput?.click()}
                    >{uploadingFiles > 0
                      ? "Uploading..."
                      : "Upload files"}</Button.Root
                  >
                </div>
                <input
                  class="brand-visually-hidden"
                  type="file"
                  autocomplete="off"
                  multiple
                  bind:this={fileInput}
                  onchange={(event) => void uploadFiles(event.currentTarget)}
                />{#if filesStatus === "checking"}<p class="brand-empty">
                    Loading files...
                  </p>{:else if filesStatus === "unavailable"}<p
                    class="brand-empty"
                  >
                    Files could not be loaded.
                  </p>
                  <Button.Root
                    class="brand-button brand-button--primary brand-button--compact"
                    type="button"
                    onclick={() =>
                      void loadFiles(
                        generation,
                        currentRoute.workspaceID,
                        currentRoute.projectID,
                        abortController!.signal,
                      )}>Try again</Button.Root
                  >{:else}{#each files.slice(0, 5) as file (file.id)}<RouterLink
                      class="brand-dashboard-row"
                      href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj/${encodeURIComponent(project.id)}/files/${encodeURIComponent(file.id)}`}
                      ><span class="brand-file-content"
                          ><span>{file.name}</span><span class="brand-file-meta"
                            ><time datetime={file.created_at}
                              >{dateLabel(file.created_at)}</time
                            ><span>{file.size} bytes</span
                            >{#if file.media_type !== undefined}<span
                                >{file.media_type}</span
                              >{/if}</span
                          ></span
                        ></RouterLink
                    >{:else}<p class="brand-empty">
                      No files yet.
                    </p>{/each}{/if}<RouterLink
                    class="brand-view-all"
                    href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj/${encodeURIComponent(project.id)}/files`}
                    >View all files</RouterLink
                  >{#if filesError !== ""}<p
                    class="brand-error"
                    aria-live="polite"
                  >
                    {filesError}
                  </p>{/if}
              </section>
              <section class="brand-dashboard-card">
                <div class="brand-card-heading">
                  <h2 class="brand-card-title">Project Notes</h2>
                  <RouterLink
                    class="brand-button brand-button--primary brand-button--compact"
                    href={`${notesPath(workspace.id, project.id)}/new`}
                    >New note</RouterLink
                  >
                </div>
                {#if notesStatus === "checking"}<p class="brand-empty">
                    Loading notes...
                  </p>{:else if notesStatus === "unavailable"}<p
                    class="brand-empty"
                  >
                    Notes could not be loaded.
                  </p>
                  <Button.Root
                    class="brand-button brand-button--primary brand-button--compact"
                    type="button"
                    onclick={() =>
                      void loadNotes(
                        generation,
                        currentRoute.workspaceID,
                        currentRoute.projectID,
                        abortController!.signal,
                      )}>Try again</Button.Root
                  >{:else}{#each notes as note (note.id)}<RouterLink
                      class="brand-dashboard-row"
                      href={notePath(workspace.id, project.id, note.id)}
                      ><span class="brand-row-content"
                        ><span
                          >{note.title}{#if note.sensitive}<span
                              class="brand-badge">Sensitive</span
                            >{/if}</span
                        >{#if note.description !== ""}<span
                            class="brand-row-description"
                            >{note.description}</span
                          >{/if}<span class="brand-row-meta"
                          ><time datetime={note.created_at}
                            >{dateLabel(note.created_at)}</time
                          ></span
                        ></span
                      ></RouterLink
                    >{:else}<p class="brand-empty">
                      No notes yet.
                    </p>{/each}{/if}<RouterLink
                  class="brand-view-all"
                  href={notesPath(workspace.id, project.id)}
                  >View all notes</RouterLink
                >
              </section>
              <section class="brand-dashboard-card">
                <div class="brand-card-heading">
                  <h2 class="brand-card-title">Project Tasks</h2>
                  <RouterLink
                    class="brand-button brand-button--primary brand-button--compact"
                    href={`${tasksPath(workspace.id, project.id)}/new`}
                    >New task</RouterLink
                  >
                </div>
                {#if tasksStatus === "checking"}<p class="brand-empty">
                    Loading tasks...
                  </p>{:else if tasksStatus === "unavailable"}<p
                    class="brand-empty"
                  >
                    Tasks could not be loaded.
                  </p>
                  <Button.Root
                    class="brand-button brand-button--primary brand-button--compact"
                    type="button"
                    onclick={() =>
                      void loadTasks(
                        generation,
                        currentRoute.workspaceID,
                        currentRoute.projectID,
                        abortController!.signal,
                      )}>Try again</Button.Root
                  >{:else}{#each tasks.slice(0, 5) as task (task.id)}<RouterLink
                      class="brand-dashboard-row"
                      href={taskPath(workspace.id, project.id, task.id)}
                      ><span class="brand-row-content"
                        ><span
                          >{task.title}
                          <span class="brand-status"
                            >{statusLabel(task.status)}</span
                          >{#if task.sensitive}<span class="brand-badge"
                              >Sensitive</span
                            >{/if}</span
                        ><span class="brand-row-meta"
                          ><time datetime={task.created_at}
                            >{dateLabel(task.created_at)}</time
                          ></span
                        ></span
                      ></RouterLink
                    >{:else}<p class="brand-empty">
                      No tasks yet.
                    </p>{/each}{/if}<RouterLink
                  class="brand-view-all"
                  href={tasksPath(workspace.id, project.id)}
                  >View all tasks</RouterLink
                >
              </section>
              <section class="brand-dashboard-card">
                <div class="brand-card-heading">
                  <h2 class="brand-card-title">Project Secrets</h2>
                  <RouterLink
                    class="brand-button brand-button--primary brand-button--compact"
                    href={`${secretsPath(workspace.id, project.id)}/new`}
                    >New secret</RouterLink
                  >
                </div>
                {#if secretsStatus === "checking"}<p class="brand-empty">
                    Loading secrets...
                  </p>{:else if secretsStatus === "unavailable"}<p
                    class="brand-empty"
                  >
                    Secrets could not be loaded.
                  </p>
                  <Button.Root
                    class="brand-button brand-button--primary brand-button--compact"
                    type="button"
                    onclick={() =>
                      void loadSecrets(
                        generation,
                        currentRoute.workspaceID,
                        currentRoute.projectID,
                        abortController!.signal,
                      )}>Try again</Button.Root
                  >{:else}{#each secrets as secret (secret.id)}<RouterLink
                      class="brand-dashboard-row"
                      href={secretPath(workspace.id, project.id, secret.id)}
                      ><span class="brand-row-content"
                        ><span>{secret.description}</span><span
                          class="brand-row-meta"
                          ><span>{secret.author.name ?? secret.author.id}</span
                          ><time datetime={secret.updated_at}
                            >Updated {dateLabel(secret.updated_at)}</time
                          ></span
                        ></span
                      ></RouterLink
                    >{:else}<p class="brand-empty">
                      No secrets yet.
                    </p>{/each}{/if}<RouterLink
                  class="brand-view-all"
                  href={secretsPath(workspace.id, project.id)}
                  >View all secrets</RouterLink
                >
              </section>
              <section class="brand-dashboard-card">
                <div class="brand-card-heading">
                  <h2 class="brand-card-title">Project Records</h2>
                </div>
                {#if recordSchemasStatus === "checking"}<p class="brand-empty">
                    Loading record types...
                  </p>{:else if recordSchemasStatus === "unavailable"}<p
                    class="brand-empty"
                  >
                    Record types could not be loaded.
                  </p>
                  <Button.Root
                    class="brand-button brand-button--primary brand-button--compact"
                    type="button"
                    onclick={() =>
                      void loadRecordSchemas(
                        generation,
                        currentRoute.workspaceID,
                        currentRoute.projectID,
                        abortController!.signal,
                      )}>Try again</Button.Root
                  >{:else}{#each recordSchemas.slice(0, 5) as schema (schema.id)}<RouterLink
                      class="brand-dashboard-row"
                      href={recordSchemaPath(
                        workspace.id,
                        project.id,
                        schema.id,
                      )}
                      ><span class="brand-row-content"
                        ><span>{schema.label}</span
                        >{#if schema.description !== ""}<span
                            class="brand-row-description"
                            >{schema.description}</span
                          >{/if}</span
                      ></RouterLink
                    >{:else}<p class="brand-empty">
                      No record types yet.
                    </p>{/each}<RouterLink
                    class="brand-view-all"
                    href={recordsPath(workspace.id, project.id)}
                    >View all records</RouterLink
                  >{/if}
              </section>
            </section>
            {#if actionError !== ""}<p class="brand-error" aria-live="polite">
                {actionError}
              </p>{/if}
          </section>
        {/if}
      </PageBody></SidebarPage.Body></SidebarPage.Page
    ></SidebarPage.Root
  >
  <Dialog.Root bind:open={editOpen}
    ><Dialog.Portal
      ><Dialog.Overlay class="brand-dialog-overlay" /><Dialog.Content
        class="brand-dialog-content"
        ><form
          class="brand-dialog-form"
          onsubmit={(event) => {
            event.preventDefault()
            void saveProject()
          }}
        >
          <div class="brand-dialog-heading">
            <Dialog.Title class="brand-dialog-title">Edit project</Dialog.Title
            ><Button.Root
              class="brand-icon-button"
              type="button"
              aria-label="Close"
              onclick={closeEdit}
              ><X size={18} strokeWidth={2} aria-hidden="true" /></Button.Root
            >
          </div>
          <div class="brand-field">
            <label for="project-edit-name">Name</label><input
              class="brand-input"
              id="project-edit-name"
              autocomplete="off"
              maxlength="256"
              bind:value={editName}
            />
          </div>
          <div class="brand-field">
            <label for="project-edit-description">Description</label><textarea
              class="brand-textarea"
              id="project-edit-description"
              autocomplete="off"
              rows="4"
              maxlength="4096"
              bind:value={editDescription}></textarea>
          </div>
          {#if editError !== ""}<p class="brand-form-error" aria-live="polite">
              {editError}
            </p>{/if}
          <div class="brand-dialog-actions">
            <Button.Root
              class="brand-button"
              type="button"
              disabled={updatingProject}
              onclick={closeEdit}>Cancel</Button.Root
            ><Button.Root
              class="brand-button brand-button--primary"
              type="submit"
              disabled={updatingProject}
              >{updatingProject ? "Saving..." : "Save changes"}</Button.Root
            >
          </div>
        </form></Dialog.Content
      ></Dialog.Portal
    ></Dialog.Root
  >
{/if}
