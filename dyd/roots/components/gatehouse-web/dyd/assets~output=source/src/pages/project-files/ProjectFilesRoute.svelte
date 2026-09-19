<script lang="ts">
  import { Button } from "bits-ui"
  import {
    fetchProjectFiles,
    finishProjectFileUpload,
    projectFileDownloadPath,
    removeProjectFile,
    startProjectFileUpload,
    type ProjectFile,
  } from "../../app/project-files"
  import { fetchProject, type Project } from "../../app/projects"
  import { useRuntime } from "../../app/runtime.svelte"
  import PageBody from "../../components/PageBody.svelte"
  import PageHeading from "../../components/PageHeading.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import StatusPage from "../../components/StatusPage.svelte"
  import * as SidebarPage from "../../components/sidebar-page"
  import WorkspaceNavigation from "../../components/WorkspaceNavigation.svelte"
  import type { Route } from "../../route"

  type FilesRoute = Extract<Route, { kind: "project-files" | "project-file" }>
  type Status = "checking" | "ready" | "unavailable"

  const runtime = useRuntime()
  const { activity, access, auth } = runtime
  let project = $state<Project | null>(null)
  let projectStatus = $state<Status>("checking")
  let files = $state<ProjectFile[]>([])
  let filesStatus = $state<Status>("checking")
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

  $effect(() => activate(currentRoute))

  function activate(route: FilesRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    project = null
    projectStatus = "checking"
    files = []
    filesStatus = "checking"
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
      project = (await response.json()) as Project
      projectStatus = "ready"
      if (!(await loadFiles(route, value, signal))) return
      if (!isCurrent(value, route) || signal.aborted) return
      if (
        route.kind === "project-file" &&
        !files.some((file) => file.id === route.fileID)
      ) {
        runtime.navigate(filesPath(route.workspaceID, route.projectID), true)
        return
      }
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
        projectStatus = "unavailable"
    }
  }

  async function loadFiles(
    route: FilesRoute,
    value: number,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    if (showLoading && isCurrent(value, route)) filesStatus = "checking"
    try {
      const response = await fetchProjectFiles(
        route.workspaceID,
        route.projectID,
        signal,
      )
      if (!isCurrent(value, route) || signal.aborted) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error()
      files = sortByCreated((await response.json()) as ProjectFile[])
      filesStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, route) && !signal.aborted)
        filesStatus = "unavailable"
      return false
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
    if (started.status === 401) {
      runtime.requireLogin()
      throw new Error()
    }
    if (!started.ok) throw new Error()
    const upload = (await started.json()) as {
      file: ProjectFile
      upload_url: string
    }
    const put = await fetch(upload.upload_url, {
      method: "PUT",
      body: file,
      ...(file.type === "" ? {} : { headers: { "Content-Type": file.type } }),
      signal,
    })
    if (!put.ok) throw new Error()
    const finished = await finishProjectFileUpload(
      route.workspaceID,
      route.projectID,
      upload.file.id,
      signal,
    )
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
  <StatusPage title="No workspace access" />
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar
      ><WorkspaceNavigation
        {workspace}
        active="projects"
      /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header>
        <SidebarPage.Toggle />
        <nav aria-label="Breadcrumb">
          <ol>
            <li><RouterLink href={`/app/wsp/${encodeURIComponent(workspace.id)}`}>{workspace.name ?? workspace.id}</RouterLink></li>
            <li><RouterLink href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj`}>Projects</RouterLink></li>
            <li><RouterLink href={projectPath(workspace.id, currentRoute.projectID)}>{project?.name ?? "Project"}</RouterLink></li>
            {#if currentRoute.kind === "project-file"}
              <li><RouterLink href={filesPath(workspace.id, currentRoute.projectID)}>Files</RouterLink></li>
              <li aria-current="page">{selectedFile?.name ?? "File"}</li>
            {:else}
              <li aria-current="page">Files</li>
            {/if}
          </ol>
        </nav>
      </SidebarPage.Header>
      <SidebarPage.Body
        ><PageBody fluid>
          {#if projectStatus === "checking"}<p class="notice">
              Loading project...
            </p>
          {:else if projectStatus === "unavailable"}<p class="notice">
              Project unavailable.
            </p>
            <button
              class="primary"
              type="button"
              onclick={() =>
                void loadRoute(
                  currentRoute,
                  generation,
                  abortController!.signal,
                )}>Try again</button
            >
           {:else if currentRoute.kind === "project-file"}<PageHeading
               as="header"
               ><p class="eyebrow">Project file</p>
               <h1>{selectedFile?.name ?? "File"}</h1></PageHeading
            >
            {#if filesStatus === "checking"}<p class="notice">
                Loading file...
              </p>
            {:else if filesStatus === "unavailable"}<p class="notice">
                File could not be loaded.
              </p>
              <Button.Root
                class="primary small"
                type="button"
                onclick={() =>
                  void loadFiles(
                    currentRoute,
                    generation,
                    abortController!.signal,
                  )}>Try again</Button.Root
              >
            {:else if selectedFile !== null}<section class="card surface stack">
                <div class="split">
                  <h2>File details</h2>
                  <div class="cluster">
                    <div>
                      <a
                        class="primary"
                        href={projectFileDownloadPath(
                          workspace.id,
                          currentRoute.projectID,
                          selectedFile.id,
                        )}
                        target="_blank"
                        rel="noopener">Download</a
                      >
                    </div>
                    <div>
                      <Button.Root
                        class="small"
                        type="button"
                        disabled={removingFileIDs.has(selectedFile.id)}
                        onclick={() => void deleteFile(selectedFile)}
                        >{removingFileIDs.has(selectedFile.id)
                          ? "Removing..."
                          : "Remove"}</Button.Root
                      >
                    </div>
                  </div>
                </div>
                <dl>
                  <div class="list-item surface up">
                    <div class="stack" style:--space="calc(var(--space) / 4)">
                      <dt>
                        <small>Name</small>
                      </dt>
                      <dd>{selectedFile.name}</dd>
                    </div>
                  </div>
                  <div class="list-item surface up">
                    <div class="stack" style:--space="calc(var(--space) / 4)">
                      <dt>
                        <small>Created</small>
                      </dt>
                      <dd>
                        <time datetime={selectedFile.created_at}
                          >{dateLabel(selectedFile.created_at)}</time
                        >
                      </dd>
                    </div>
                  </div>
                  <div class="list-item surface up">
                    <div class="stack" style:--space="calc(var(--space) / 4)">
                      <dt>
                        <small>Size</small>
                      </dt>
                      <dd>{selectedFile.size} bytes</dd>
                    </div>
                  </div>
                  <div class="list-item surface up">
                    <div class="stack" style:--space="calc(var(--space) / 4)">
                      <dt>
                        <small>Media type</small>
                      </dt>
                      <dd>
                        {selectedFile.media_type ?? "Unknown"}
                      </dd>
                    </div>
                  </div>
                  <div class="list-item surface up">
                    <div class="stack" style:--space="calc(var(--space) / 4)">
                      <dt>
                        <small>Fingerprint</small>
                      </dt>
                      <dd>{selectedFile.fingerprint}</dd>
                    </div>
                  </div>
                </dl>
              </section>{/if}
            {#if filesError !== ""}<p class="notice action" aria-live="polite">
                {filesError}
              </p>{/if}
           {:else}<PageHeading as="header"
               ><p class="eyebrow">Project files</p>
               <h1>Files</h1></PageHeading
            >
            <section class="card surface stack">
              <div class="split">
                <h2>Project Files</h2>
                <Button.Root
                  class="primary small"
                  type="button"
                  disabled={uploadingFiles > 0}
                  onclick={() => fileInput?.click()}
                  >{uploadingFiles > 0
                    ? "Uploading..."
                    : "Upload files"}</Button.Root
                >
              </div>
              <input
                class="visually-hidden"
                type="file"
                autocomplete="off"
                multiple
                bind:this={fileInput}
                onchange={(event) => void uploadFiles(event.currentTarget)}
              />
              {#if filesStatus === "checking"}<p class="notice">
                  Loading files...
                </p>
              {:else if filesStatus === "unavailable"}<p class="notice">
                  Files could not be loaded.
                </p>
                <Button.Root
                  class="primary small"
                  type="button"
                  onclick={() =>
                    void loadFiles(
                      currentRoute,
                      generation,
                      abortController!.signal,
                    )}>Try again</Button.Root
                >
              {:else}{#each files as file (file.id)}<RouterLink
                    class="list-item surface up"
                    href={filePath(
                      workspace.id,
                      currentRoute.projectID,
                      file.id,
                    )}
                    ><span class="stack" style:--space="calc(var(--space) / 4)"
                      ><span>{file.name}</span><span class="cluster" style:--space="calc(var(--space) / 2)"
                        ><time datetime={file.created_at}
                          >{dateLabel(file.created_at)}</time
                        ><span>{file.size} bytes</span
                        >{#if file.media_type !== undefined}<span
                            >{file.media_type}</span
                          >{/if}</span
                      ></span
                    ></RouterLink
                  >{:else}<p class="notice">No files yet.</p>{/each}{/if}
              {#if filesError !== ""}<p class="notice action" aria-live="polite">
                  {filesError}
                </p>{/if}
            </section>
          {/if}
        </PageBody></SidebarPage.Body
      >
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
