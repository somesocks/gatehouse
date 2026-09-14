<script lang="ts">
  import { Menu } from "@lucide/svelte"
  import {
    chatFileDownloadPath,
    fetchChatFiles,
    fetchChatSession,
    type ChatFile,
  } from "../../app/chat"
  import { useRuntime } from "../../app/runtime.svelte"
  import PageBody from "../../components/PageBody.svelte"
  import PageHeading from "../../components/PageHeading.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import SessionNavigation from "../../components/SessionNavigation.svelte"
  import * as SidebarPage from "../../components/sidebar-page"
  import WorkspaceNavigation from "../../components/WorkspaceNavigation.svelte"
  import type { Route } from "../../route"

  type FilesRoute = Extract<Route, { kind: "session-files" }>
  type Session = {
    id: string
    created_at: string
    name?: string
    project?: { id: string; name?: string }
  }
  type Status = "checking" | "ready" | "unavailable"

  const runtime = useRuntime()
  const { access, activity, auth } = runtime
  let session = $state<Session | null>(null)
  let sessionStatus = $state<Status>("checking")
  let files = $state<ChatFile[]>([])
  let filesStatus = $state<Status>("checking")
  let generation = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as FilesRoute)
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const workspacePath = (workspaceID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}`
  const sessionsPath = (workspaceID: string) =>
    `${workspacePath(workspaceID)}/ses`
  const sessionPath = (workspaceID: string, sessionID: string) =>
    `${sessionsPath(workspaceID)}/${encodeURIComponent(sessionID)}`
  const projectPath = (workspaceID: string, projectID: string) =>
    `${workspacePath(workspaceID)}/prj/${encodeURIComponent(projectID)}`
  const isCurrent = (
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
  ) =>
    value === generation &&
    !signal.aborted &&
    currentRoute.workspaceID === workspaceID &&
    currentRoute.sessionID === sessionID

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })

  function activate(route: FilesRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    session = null
    sessionStatus = "checking"
    files = []
    filesStatus = "checking"
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
    route: FilesRoute,
    value: number,
    signal: AbortSignal,
  ): Promise<void> {
    const { workspaceID, sessionID } = route
    if (
      !access.state.workspaces.some((candidate) => candidate.id === workspaceID)
    ) {
      if (isCurrent(value, workspaceID, sessionID, signal))
        runtime.navigate(
          access.state.workspaces.length === 0
            ? "/app/no-access"
            : workspacePath(access.state.workspaces[0].id),
          true,
        )
      return
    }
    if (!(await loadSession(value, workspaceID, sessionID, signal))) return
    if (!(await loadFiles(value, workspaceID, sessionID, signal))) return
    if (!isCurrent(value, workspaceID, sessionID, signal)) return
    unsubscribe = activity.subscribe(
      [
        {
          name: "session-files",
          topic: `${workspaceID}/${sessionID}`,
          events: ["session.*"],
        },
      ],
      async ({ signal: pollSignal }) => {
        if (
          pollSignal.aborted ||
          !isCurrent(value, workspaceID, sessionID, signal)
        )
          return
        if (!(await loadFiles(value, workspaceID, sessionID, signal, false)))
          throw new Error("session files refresh failed")
      },
    )
    void activity.poll()
  }

  async function loadSession(
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
  ): Promise<boolean> {
    try {
      const response = await fetchChatSession(workspaceID, sessionID, signal)
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (response.status === 404) {
        runtime.navigate(sessionsPath(workspaceID), true)
        return false
      }
      if (!response.ok) throw new Error("session unavailable")
      session = (await response.json()) as Session
      sessionStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        sessionStatus = "unavailable"
      return false
    }
  }

  async function loadFiles(
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, sessionID, signal))
      filesStatus = "checking"
    try {
      const response = await fetchChatFiles(workspaceID, sessionID, signal)
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("session files unavailable")
      files = (await response.json()) as ChatFile[]
      filesStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        filesStatus = "unavailable"
      return false
    }
  }

  function sizeLabel(size: number): string {
    if (!Number.isFinite(size) || size < 0) return "Unknown size"
    const units = ["B", "KiB", "MiB", "GiB", "TiB"]
    let value = size
    let unit = 0
    while (value >= 1024 && unit < units.length - 1) {
      value /= 1024
      unit += 1
    }
    return `${value >= 10 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <main class="status-page" aria-busy="true" aria-live="polite">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <div class="loading-mark" aria-hidden="true"></div>
      <p>Loading your workspace.</p>
    </section>
  </main>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <main class="status-page">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">Connection unavailable</h1>
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
      <button
        class="button is-primary"
        type="button"
        onclick={() => runtime.navigate("/app/no-access", true)}
        >Continue</button
      >
    </section>
  </main>
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar
      ><WorkspaceNavigation {workspace} active="chats" /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header>
        <SidebarPage.Toggle
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
            href={workspacePath(workspace.id)}
            ><span>{workspace.name ?? workspace.id}</span></RouterLink
          >
          {#if session !== null}{#if session.project !== undefined}<span
                class="brand-workspace-breadcrumb-separator"
                aria-hidden="true">/</span
              ><RouterLink
                class="brand-workspace-breadcrumb-segment"
                href={projectPath(workspace.id, session.project.id)}
                ><span>{session.project.name ?? "New Project"}</span
                ></RouterLink
              >{/if}<span
              class="brand-workspace-breadcrumb-separator"
              aria-hidden="true">/</span
            ><RouterLink
              class="brand-workspace-breadcrumb-segment"
              href={sessionPath(workspace.id, session.id)}
              ><span>{session.name ?? "New Chat"}</span></RouterLink
            ><span
              class="brand-workspace-breadcrumb-separator"
              aria-hidden="true">/</span
            ><span>Files</span>{/if}
        </h1>
        {#if session !== null}<SessionNavigation
            workspaceID={workspace.id}
            sessionID={session.id}
            active="files"
          />{/if}
      </SidebarPage.Header>
      <SidebarPage.Body>
        {#if sessionStatus === "checking"}
          <p class="dashboard-empty" aria-busy="true" aria-live="polite">
            Loading chat...
          </p>
        {:else if sessionStatus === "unavailable"}
          <p class="dashboard-empty">This chat could not be loaded.</p>
        {:else}
          <PageBody>
            <PageHeading>
              <h2>Files</h2>
            </PageHeading>
            <div class="collection-list">
              {#if filesStatus === "checking"}
                <p class="dashboard-empty" aria-busy="true">Loading files...</p>
              {:else if filesStatus === "unavailable"}
                <p class="dashboard-empty">Files could not be loaded.</p>
              {:else}
                {#each files as file (file.id)}
                  <article class="dashboard-row project-note-row">
                    <span class="dashboard-row-content"
                      ><span class="project-note-title">{file.name}</span><span
                        class="dashboard-row-meta"
                        ><span>{file.media_type ?? "Unknown type"}</span><span
                          >{sizeLabel(file.size)}</span
                        ></span
                      ></span
                    >
                    <a
                      class="button is-small"
                      href={chatFileDownloadPath(
                        workspace.id,
                        session?.id ?? "",
                        file.id,
                      )}
                      target="_blank"
                      rel="noopener">Download</a
                    >
                  </article>
                {:else}
                  <p class="dashboard-empty">
                    No files attached to this session.
                  </p>
                {/each}
              {/if}
            </div>
          </PageBody>
        {/if}
      </SidebarPage.Body>
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
