<script lang="ts">
  import { untrack } from "svelte"
  import { createLoadState } from "../../utils/load-state.svelte"
  import { PageBody, PageHeading, RouterLink, StatusPage } from "../../ui"
  import {
    workspaceNavigation,
    sessionViews,
    renameSession,
  } from "../../app/navigation"
  import * as UI from "../../ui"
  import SessionViewNavigation from "../../session-participants/SessionViewNavigation.svelte"
  import { createCollectionPage } from "../../utils/collection-page.svelte"
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

  let { embedded = false }: { embedded?: boolean } = $props()
  const runtime = useRuntime()
  const sessionLoad = createLoadState()
  const filesLoad = createLoadState()
  const page = createCollectionPage()
  const reloadFiles = () =>
    loadFiles(
      generation,
      currentRoute.workspaceID,
      currentRoute.sessionID,
      abortController!.signal,
    )
  const { access, activity, auth } = runtime
  let session = $state<Session | null>(null)
  const sessionStatus = $derived(sessionLoad.status)
  let files = $state<ChatFile[]>([])
  let includeAgentFiles = $state(false)
  const filesStatus = $derived(filesLoad.status)
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
    includeAgentFiles = false
    page.reset()
    sessionLoad.reset()
    filesLoad.reset()
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    session = null
    files = []
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
        sessionLoad.stop()
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
    unsubscribe?.()
    unsubscribe = activity.subscribe(
      [
        {
          name: "session-files",
          topic: `${workspaceID}/${sessionID}`,
          events: ["session.*", "session_file.*"],
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
    const request = sessionLoad.begin(signal)
    signal = request.signal
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
      const loaded = (await response.json()) as Session
      return request.commit(() => {
        session = loaded
      })
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        request.fail(new Error("Chat could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  async function loadFiles(
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    const request = filesLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
    try {
      const requested = `${page.key()}:${includeAgentFiles}`
      const response = await fetchChatFiles(
        workspaceID,
        sessionID,
        signal,
        page.params(),
        { author: includeAgentFiles ? "all" : "principal" },
      )
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("session files unavailable")
      const loaded = (await response.json()) as ChatFile[]
      if (
        !isCurrent(value, workspaceID, sessionID, signal) ||
        requested !== `${page.key()}:${includeAgentFiles}`
      )
        return false
      return request.commit(() => {
        files = loaded
        page.accept(response)
      })
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        request.fail(new Error("Files could not be loaded."))
      return false
    } finally {
      request.finish()
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
  <UI.ResourcePage {embedded}>
    {#snippet sidebar()}<UI.Navigation
        {...workspaceNavigation(runtime, workspace, "chats")}
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
        {#if session !== null}<SessionViewNavigation
            workspaceID={workspace.id}
            sessionID={session.id}
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
    {/snippet}
        {#if filesLoad.reloading || sessionLoad.reloading}<UI.Text role="status"
            >Refreshing files…</UI.Text
          >{/if}
        {#if filesLoad.state.error || sessionLoad.state.error}<UI.Notice
            variant="destructive"
            role="alert"
            >{filesLoad.state.error || sessionLoad.state.error}<UI.Button
              variant="outline"
              onclick={() =>
                void loadRoute(
                  currentRoute,
                  generation,
                  abortController!.signal,
                )}>Retry</UI.Button
            ></UI.Notice
          >{/if}
        {#if sessionStatus === "checking"}
          <UI.Text as="p" tone="muted" aria-busy="true" aria-live="polite">
            Loading chat...
          </UI.Text>
        {:else if sessionStatus === "unavailable"}
          <UI.Text as="p" tone="muted">This chat could not be loaded.</UI.Text>
        {:else}
          <PageBody fluid>
            {#if !embedded}<UI.AdminHeader
              title="Files"
              description="Inspect and download chat attachments."
            />{/if}
            {#if filesStatus === "unavailable"}<UI.Notice
                variant="destructive"
                role="alert">Files could not be loaded.</UI.Notice
              ><UI.Button variant="outline" onclick={() => void reloadFiles()}
                >Try again</UI.Button
              >{/if}
            <UI.RecordList
              label="Files"
              items={files.map((file) => ({ ...file, label: file.name }))}
              loading={page.state.busy || filesLoad.state.pending}
              scrollable={false}
              pageKey={JSON.stringify([page.state.query, page.state.cursor, includeAgentFiles])}
              emptyMessage={filesStatus === "checking" ? "Loading…" : filesStatus === "unavailable" ? "Files could not be loaded." : "No matching files."}
            >
              {#snippet toolbar()}
                <UI.Stack gap="small">
                  <UI.SearchToolbar framed={false} label="Search files" placeholder="Search" bind:value={page.state.search} loading={page.state.busy || filesLoad.state.pending} onsearch={() => void page.search(reloadFiles)} />
                  <UI.Label layout="choice">
                    <UI.Checkbox checked={includeAgentFiles} onCheckedChange={(checked) => { includeAgentFiles = checked; page.reset(); void reloadFiles() }} />
                    Include agent-authored files
                  </UI.Label>
                </UI.Stack>
              {/snippet}
              {#snippet details(file)}{file.media_type ?? "Unknown type"} · {sizeLabel(file.size)}{/snippet}
              {#snippet actions(file)}
                <UI.Link variant="ghost" size="icon" href={chatFileDownloadPath(workspace.id, currentRoute.sessionID, file.id)} target="_blank" rel="noopener" aria-label={`Download ${file.name}`} title="Download">
                  <UI.Download size={16} aria-hidden="true" />
                </UI.Link>
              {/snippet}
            </UI.RecordList>
            {#if page.state.history.length > 0 || page.state.next}<UI.PaginationFooter
              showCount={false}
              count={files.length}
              previous={page.state.history.length > 0}
              next={!!page.state.next}
              loading={page.state.busy || filesLoad.state.pending}
              onprevious={() => void page.previous(reloadFiles)}
              onnext={() => void page.next(reloadFiles)}
            />{/if}
          </PageBody>
        {/if}
  </UI.ResourcePage>
{/if}
