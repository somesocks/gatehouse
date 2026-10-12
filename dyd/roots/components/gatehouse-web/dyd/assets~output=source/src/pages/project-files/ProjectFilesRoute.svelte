<script lang="ts">
  import { untrack } from "svelte"
  import { createLoadState } from "../../utils/load-state.svelte"
  import { PageBody, PageHeading, RouterLink, StatusPage } from "../../ui"
  import { workspaceNavigation } from "../../app/navigation"
  import * as UI from "../../ui"
  import { createCollectionPage } from "../../utils/collection-page.svelte"
  import { formatFileSize } from "../../utils/file-size"

  import {
    fetchProjectFiles,
    fetchProjectFile,
    finishProjectFileUpload,
    projectFileDownloadPath,
    removeProjectFile,
    startProjectFileUpload,
    type ProjectFile,
  } from "../../app/project-files"
  import { fetchProject, type Project } from "../../app/projects"
  import { useRuntime } from "../../app/runtime.svelte"
  import * as SidebarPage from "../../ui/sidebar-page"

  import type { Route } from "../../route"

  type FilesRoute = Extract<Route, { kind: "project-files" | "project-file" }>
  type Status = "checking" | "ready" | "unavailable"

  const runtime = useRuntime()
  const projectLoad = createLoadState()
  const filesLoad = createLoadState()
  const page = createCollectionPage()
  const reloadFiles = () =>
    loadFiles(currentRoute, generation, abortController!.signal)
  const { activity, access, auth } = runtime
  let project = $state<Project | null>(null)
  const projectStatus = $derived(projectLoad.status)
  let files = $state<ProjectFile[]>([])
  const filesStatus = $derived(filesLoad.status)
  let filesError = $state("")
  let uploadingFiles = $state(0)
  let removingFileIDs = $state<Set<string>>(new Set())
  let fileInput = $state<HTMLInputElement | undefined>()
  let generation = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as FilesRoute)
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const projectPath = (workspaceID: string, projectID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}/prj/${encodeURIComponent(projectID)}`
  const filesPath = (workspaceID: string, projectID: string) =>
    `${projectPath(workspaceID, projectID)}/files`
  const filePath = (workspaceID: string, projectID: string, fileID: string) =>
    `${filesPath(workspaceID, projectID)}/${encodeURIComponent(fileID)}`
  const selectedFile = $derived(
    currentRoute.kind === "project-file"
      ? (files.find((file) => file.id === currentRoute.fileID) ?? null)
      : null,
  )
  const sortByCreated = <T extends { id: string; created_at: string }>(
    items: T[],
  ): T[] =>
    [...items].sort(
      (left, right) =>
        Date.parse(right.created_at) - Date.parse(left.created_at) ||
        right.id.localeCompare(left.id),
    )
  const isCurrent = (value: number, route: FilesRoute) =>
    value === generation &&
    currentRoute.workspaceID === route.workspaceID &&
    currentRoute.projectID === route.projectID &&
    !abortController?.signal.aborted
  const dateLabel = (value: string) => {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const part = (number: number) => number.toString().padStart(2, "0")
    return `${date.getFullYear()}-${part(date.getMonth() + 1)}-${part(date.getDate())} ${part(date.getHours())}:${part(date.getMinutes())}`
  }

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
    return untrack(() => activate(currentRoute))
  })

  function activate(route: FilesRoute): () => void {
    page.reset()
    projectLoad.reset()
    filesLoad.reset()
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    project = null
    files = []
    filesError = ""
    uploadingFiles = 0
    removingFileIDs = new Set()
    if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "ready"
    )
      void loadRoute(route, value, abortController.signal)
    else if (auth.state.status === "authenticated") void runtime.refresh()
    return () => {
      if (value === generation) {
        abortController?.abort()
        projectLoad.stop()
        filesLoad.stop()
        unsubscribe?.()
        unsubscribe = undefined
      }
    }
  }

  async function loadRoute(
    route: FilesRoute,
    value: number,
    signal: AbortSignal,
  ): Promise<void> {
    if (
      !access.state.workspaces.some((item) => item.id === route.workspaceID)
    ) {
      runtime.navigate(
        access.state.workspaces.length === 0
          ? "/app/no-access"
          : `/app/wsp/${encodeURIComponent(access.state.workspaces[0].id)}`,
        true,
      )
      return
    }
    const request = projectLoad.begin(signal)
    signal = request.signal
    try {
      const response = await fetchProject(
        route.workspaceID,
        route.projectID,
        signal,
      )
      if (!isCurrent(value, route) || signal.aborted) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (response.status === 404) {
        runtime.navigate(
          `/app/wsp/${encodeURIComponent(route.workspaceID)}/prj`,
          true,
        )
        return
      }
      if (!response.ok) throw new Error()
      const loaded = (await response.json()) as Project
      if (
        !request.commit(() => {
          project = loaded
        })
      )
        return
      if (!(await loadFiles(route, value, signal))) return
      if (!isCurrent(value, route) || signal.aborted) return
      if (
        route.kind === "project-file" &&
        !files.some((file) => file.id === route.fileID)
      ) {
        runtime.navigate(filesPath(route.workspaceID, route.projectID), true)
        return
      }
      unsubscribe?.()
      unsubscribe = activity.subscribe(
        [
          {
            name: "project-files",
            topic: `${route.workspaceID}/${route.projectID}`,
            events: ["project_file.*"],
          },
        ],
        async ({ signal: activitySignal }) => {
          if (!isCurrent(value, route) || activitySignal.aborted) return
          if (!(await loadFiles(route, value, activitySignal, false)))
            throw new Error("project files refresh failed")
          if (
            route.kind === "project-file" &&
            !files.some((file) => file.id === route.fileID)
          )
            runtime.navigate(
              filesPath(route.workspaceID, route.projectID),
              true,
            )
        },
      )
      void activity.poll()
    } catch {
      if (isCurrent(value, route) && !signal.aborted)
        request.fail(new Error("Project could not be loaded."))
    } finally {
      request.finish()
    }
  }

  async function loadFiles(
    route: FilesRoute,
    value: number,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    const request = filesLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
    try {
      const requested = page.key()
      const response =
        route.kind === "project-file"
          ? await fetchProjectFile(
              route.workspaceID,
              route.projectID,
              route.fileID,
              signal,
            )
          : await fetchProjectFiles(
              route.workspaceID,
              route.projectID,
              signal,
              page.params(),
            )
      if (!isCurrent(value, route) || signal.aborted) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (response.status === 404 && route.kind === "project-file") {
        runtime.navigate(filesPath(route.workspaceID, route.projectID), true)
        return false
      }
      if (!response.ok) throw new Error()
      const result = (await response.json()) as ProjectFile | ProjectFile[]
      if (
        !isCurrent(value, route) ||
        requested !== page.key() ||
        signal.aborted
      )
        return false
      return request.commit(() => {
        files = Array.isArray(result) ? result : [result]
        page.accept(response)
      })
    } catch {
      if (isCurrent(value, route) && !signal.aborted)
        request.fail(new Error("Files could not be loaded."))
      return false
    } finally {
      request.finish()
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
          uploadFile(route, file, abortController?.signal),
        ),
      )
      if (!isCurrent(value, route)) return
      await loadFiles(route, value, abortController!.signal, false)
      if (!isCurrent(value, route)) return
      if (results.some((result) => result.status === "rejected"))
        filesError = "Some files could not be uploaded. Try again."
    } finally {
      if (isCurrent(value, route)) uploadingFiles -= selected.length
    }
  }

  async function uploadFile(
    route: FilesRoute,
    file: File,
    signal?: AbortSignal,
  ): Promise<void> {
    const started = await startProjectFileUpload(
      route.workspaceID,
      route.projectID,
      file,
      signal,
    )
    signal?.throwIfAborted()
    if (started.status === 401) {
      runtime.requireLogin()
      throw new Error()
    }
    if (!started.ok) throw new Error()
    const upload = (await started.json()) as {
      file: ProjectFile
      upload_url: string
    }
    signal?.throwIfAborted()
    const put = await fetch(upload.upload_url, {
      method: "PUT",
      body: file,
      ...(file.type === "" ? {} : { headers: { "Content-Type": file.type } }),
      signal,
    })
    signal?.throwIfAborted()
    if (!put.ok) throw new Error()
    const finished = await finishProjectFileUpload(
      route.workspaceID,
      route.projectID,
      upload.file.id,
      signal,
    )
    signal?.throwIfAborted()
    if (finished.status === 401) {
      runtime.requireLogin()
      throw new Error()
    }
    if (!finished.ok) throw new Error()
  }

  async function deleteFile(file: ProjectFile): Promise<void> {
    if (removingFileIDs.has(file.id) || !window.confirm(`Remove ${file.name}?`))
      return
    const route = currentRoute
    const value = generation
    removingFileIDs = new Set(removingFileIDs).add(file.id)
    filesError = ""
    try {
      const response = await removeProjectFile(
        route.workspaceID,
        route.projectID,
        file.id,
        abortController?.signal,
      )
      if (!isCurrent(value, route)) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error()
      files = files.filter((candidate) => candidate.id !== file.id)
      if (route.kind === "project-file")
        runtime.navigate(filesPath(route.workspaceID, route.projectID), true)
    } catch {
      if (isCurrent(value, route))
        filesError = "The file could not be removed. Try again."
    } finally {
      if (isCurrent(value, route)) {
        const next = new Set(removingFileIDs)
        next.delete(file.id)
        removingFileIDs = next
      }
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
  <StatusPage title="No workspace access" />
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar
      ><UI.Navigation
        {...workspaceNavigation(runtime, workspace, "projects")}
      /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header>
        <SidebarPage.Toggle />
        <UI.Breadcrumb.Root aria-label="Breadcrumb">
          <UI.Breadcrumb.List>
            <UI.Breadcrumb.Item
              ><RouterLink href={`/app/wsp/${encodeURIComponent(workspace.id)}`}
                >{workspace.name ?? "Workspace"}</RouterLink
              ></UI.Breadcrumb.Item
            >
            <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
              ><RouterLink
                href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj`}
                >Projects</RouterLink
              ></UI.Breadcrumb.Item
            >
            <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
              ><RouterLink
                href={projectPath(workspace.id, currentRoute.projectID)}
                >{project?.name ?? "Project"}</RouterLink
              ></UI.Breadcrumb.Item
            >
            {#if currentRoute.kind === "project-file"}
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                ><RouterLink
                  href={filesPath(workspace.id, currentRoute.projectID)}
                  >Files</RouterLink
                ></UI.Breadcrumb.Item
              >
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
                ><UI.Breadcrumb.Page
                  >{selectedFile?.name ?? "File"}</UI.Breadcrumb.Page
                ></UI.Breadcrumb.Item
              >
            {:else}
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
                ><UI.Breadcrumb.Page>Files</UI.Breadcrumb.Page
                ></UI.Breadcrumb.Item
              >
            {/if}
          </UI.Breadcrumb.List>
        </UI.Breadcrumb.Root>
      </SidebarPage.Header>
      <SidebarPage.Body
        ><PageBody fluid>
          {#if filesLoad.reloading || projectLoad.reloading}<UI.Text
              role="status">Refreshing files…</UI.Text
            >{/if}
          {#if filesLoad.state.error || projectLoad.state.error}<UI.Notice
              variant="destructive"
              role="alert"
              >{filesLoad.state.error || projectLoad.state.error}<UI.Button
                variant="outline"
                onclick={() => void reloadFiles()}>Retry</UI.Button
              ></UI.Notice
            >{/if}
          {#if projectStatus === "checking"}<UI.Notice>
              Loading project...
            </UI.Notice>
          {:else if projectStatus === "unavailable"}<UI.Notice>
              Project unavailable.
            </UI.Notice>
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
          {:else if currentRoute.kind === "project-file"}
            {#snippet fileActions()}{#if selectedFile}<UI.Link
                  variant="default"
                  href={projectFileDownloadPath(
                    workspace.id,
                    currentRoute.projectID,
                    selectedFile.id,
                  )}
                  target="_blank"
                  rel="noopener">Download</UI.Link
                ><UI.Button
                  variant="outline"
                  disabled={removingFileIDs.has(selectedFile.id)}
                  onclick={() => selectedFile && void deleteFile(selectedFile)}
                  >{removingFileIDs.has(selectedFile.id)
                    ? "Removing…"
                    : "Remove"}</UI.Button
                >{/if}{/snippet}
            <UI.AdminHeader
              kind="Project file"
              title={selectedFile?.name ?? "File"}
              actions={fileActions}
            />
            {#if filesStatus === "checking"}<UI.Notice>
                Loading file...
              </UI.Notice>
            {:else if filesStatus === "unavailable"}<UI.Notice>
                File could not be loaded.
              </UI.Notice>
              <UI.Button
                variant="default"
                size="sm"
                type="button"
                onclick={() =>
                  void loadFiles(
                    currentRoute,
                    generation,
                    abortController!.signal,
                  )}>Try again</UI.Button
              >
            {:else if selectedFile !== null}<UI.DetailSection
                title="File information"
              >
                <UI.Property label="Created"
                  >{dateLabel(selectedFile.created_at)}</UI.Property
                >
                <UI.Property label="Size"
                  >{formatFileSize(selectedFile.size)}</UI.Property
                >
                <UI.Property label="Media type"
                  >{selectedFile.media_type ?? "Unknown"}</UI.Property
                >
              </UI.DetailSection>{/if}
            {#if filesError !== ""}<UI.Notice
                variant="destructive"
                aria-live="polite"
              >
                {filesError}
              </UI.Notice>{/if}
          {:else}{#snippet uploadActions()}
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
            {/snippet}
            <UI.AdminHeader
              title="Files"
              description="Upload, inspect, and download project files."
              actions={uploadActions}
            />
            <UI.SearchToolbar
              label="Search files"
              placeholder="File name or media type"
              bind:value={page.state.search}
              loading={page.state.busy || filesLoad.state.pending}
              onsearch={() => void page.search(reloadFiles)}
            />
            <UI.FileInput
              hidden
              type="file"
              autocomplete="off"
              multiple
              bind:ref={fileInput}
              onchange={(event) => void uploadFiles(event.currentTarget)}
            />
            {#if filesStatus === "checking"}<UI.Notice>
                Loading files...
              </UI.Notice>
            {:else if filesStatus === "unavailable"}<UI.Notice>
                Files could not be loaded.
              </UI.Notice>
              <UI.Button
                variant="default"
                size="sm"
                type="button"
                onclick={() =>
                  void loadFiles(
                    currentRoute,
                    generation,
                    abortController!.signal,
                  )}>Try again</UI.Button
              >
            {/if}
            <UI.RecordTable headers={["Name", "Media type", "Size", "Created"]}>
              {#each files as file (file.id)}<UI.RecordRow>
                  <UI.RecordCell primary
                    ><RouterLink
                      href={filePath(
                        workspace.id,
                        currentRoute.projectID,
                        file.id,
                      )}>{file.name}</RouterLink
                    ></UI.RecordCell
                  >
                  <UI.RecordCell>{file.media_type ?? "Unknown"}</UI.RecordCell>
                  <UI.RecordCell>{formatFileSize(file.size)}</UI.RecordCell>
                  <UI.RecordCell>{dateLabel(file.created_at)}</UI.RecordCell>
                </UI.RecordRow>{:else}<UI.TableEmpty
                  columns={4}
                  message={filesStatus === "unavailable"
                    ? "Files could not be loaded."
                    : filesStatus === "checking"
                      ? "Loading files…"
                      : "No matching files."}
                />{/each}
            </UI.RecordTable>
            <UI.PaginationFooter
              noun="files"
              count={files.length}
              previous={page.state.history.length > 0}
              next={!!page.state.next}
              loading={page.state.busy || filesLoad.state.pending}
              onprevious={() => void page.previous(reloadFiles)}
              onnext={() => void page.next(reloadFiles)}
            />
            {#if filesError !== ""}<UI.Notice
                variant="destructive"
                aria-live="polite"
              >
                {filesError}
              </UI.Notice>{/if}
          {/if}
        </PageBody></SidebarPage.Body
      >
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
