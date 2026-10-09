<script lang="ts">
  import { PageBody, PageHeading, RouterLink, StatusPage } from "../../ui"
  import {
    workspaceNavigation,
    sessionViews,
    renameSession,
  } from "../../app/navigation"
  import * as UI from "../../ui"
  import {
    chatFileDownloadPath,
    fetchChatFiles,
    fetchChatSession,
    type ChatFile,
  } from "../../app/chat"
  import { useRuntime } from "../../app/runtime.svelte"

  import * as SidebarPage from "../../ui/sidebar-page"

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
  function updateSessionName(name: string): void {
    if (session !== null) session = { ...session, name }
  }

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
  <StatusPage eyebrow="Gatehouse" busy live="polite">
    {#snippet children()}
      <UI.Text as="p">Loading your workspace.</UI.Text>
    {/snippet}
  </StatusPage>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage eyebrow="Gatehouse" title="Connection unavailable">
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
  <StatusPage eyebrow="Gatehouse" title="No workspace access">
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
        {...workspaceNavigation(runtime, workspace, "chats")}
      /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header>
        <SidebarPage.Toggle />
        <UI.Breadcrumb.Root aria-label="Breadcrumb">
          <UI.Breadcrumb.List>
            <UI.Breadcrumb.Item
              ><RouterLink href={workspacePath(workspace.id)}
                >{workspace.name ?? workspace.id}</RouterLink
              ></UI.Breadcrumb.Item
            >
            {#if session !== null}
              {#if session.project !== undefined}
                <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                  ><RouterLink
                    href={projectPath(workspace.id, session.project.id)}
                    >{session.project.name ?? "New Project"}</RouterLink
                  ></UI.Breadcrumb.Item
                >
              {/if}
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                ><RouterLink href={sessionPath(workspace.id, session.id)}
                  >{session.name ?? "New Chat"}</RouterLink
                ></UI.Breadcrumb.Item
              >
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
                ><UI.Breadcrumb.Page>Files</UI.Breadcrumb.Page
                ></UI.Breadcrumb.Item
              >
            {/if}
          </UI.Breadcrumb.List>
        </UI.Breadcrumb.Root>
        {#if session !== null}<UI.ViewNavigation
            items={sessionViews(workspace.id, session.id, "files")}
            name={session.name}
            onRename={(name) =>
              renameSession(
                runtime,
                workspace.id,
                currentRoute.sessionID,
                name,
                updateSessionName,
              )}
          />{/if}
      </SidebarPage.Header>
      <SidebarPage.Body>
        {#if sessionStatus === "checking"}
          <UI.Text as="p" tone="muted" aria-busy="true" aria-live="polite">
            Loading chat...
          </UI.Text>
        {:else if sessionStatus === "unavailable"}
          <UI.Text as="p" tone="muted">This chat could not be loaded.</UI.Text>
        {:else}
          <PageBody fluid>
            <PageHeading>
              <UI.Heading level={2}>Files</UI.Heading>
            </PageHeading>
            <UI.List>
              {#if filesStatus === "checking"}
                <UI.Text as="p" tone="muted" aria-busy="true"
                  >Loading files...</UI.Text
                >
              {:else if filesStatus === "unavailable"}
                <UI.Text as="p" tone="muted">Files could not be loaded.</UI.Text
                >
              {:else}
                {#each files as file (file.id)}
                  <UI.Item>
                    <UI.Stack as="span"
                      ><UI.Text>{file.name}</UI.Text><UI.Row
                        tone="muted"
                        as="small"
                        ><UI.Text>{file.media_type ?? "Unknown type"}</UI.Text
                        ><UI.Text>{sizeLabel(file.size)}</UI.Text></UI.Row
                      ></UI.Stack
                    >
                    <UI.Link
                      size="sm"
                      href={chatFileDownloadPath(
                        workspace.id,
                        session?.id ?? "",
                        file.id,
                      )}
                      target="_blank"
                      rel="noopener">Download</UI.Link
                    >
                  </UI.Item>
                {:else}
                  <UI.Text as="p" tone="muted">
                    No files attached to this session.
                  </UI.Text>
                {/each}
              {/if}
            </UI.List>
          </PageBody>
        {/if}
      </SidebarPage.Body>
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
