<script lang="ts">
  import { untrack } from "svelte"
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
  import { useRuntime } from "../../app/runtime.svelte"
  import {
    createSessionNote,
    fetchSessionNote,
    fetchSessionNoteRevision,
    fetchSessionNoteRevisions,
    fetchSessionNotes,
    removeSessionNote,
    updateSessionNote,
    type SessionNote,
  } from "../../app/session-notes"

  import * as SidebarPage from "../../ui/sidebar-page"

  import type { Route } from "../../route"

  type NotesRoute = Extract<
    Route,
    {
      kind:
        | "session-notes"
        | "session-note-new"
        | "session-note"
        | "session-note-edit"
        | "session-note-history"
        | "session-note-revision"
    }
  >
  type Session = {
    id: string
    created_at: string
    name?: string
    project?: { id: string; name?: string }
  }
  type Status = "checking" | "ready" | "unavailable"
  type Revision = Pick<
    SessionNote,
    "title" | "description" | "sensitive" | "author" | "created_at"
  > & { revision: number; body?: string }

  let { embedded = false }: { embedded?: boolean } = $props()
  const runtime = useRuntime()
  const sessionLoad = createLoadState()
  const notesLoad = createLoadState()
  const detailLoad = createLoadState()
  const revisionLoad = createLoadState()
  const page = createCollectionPage()
  const historyPage = createCollectionPage()
  const reloadHistory = async () => {
    await openHistory(false)
    return historyError === ""
  }
  const reloadNotes = () =>
    loadNotes(
      generation,
      currentRoute.workspaceID,
      currentRoute.sessionID,
      abortController!.signal,
    )
  const { access, activity, auth } = runtime
  let session = $state<Session | null>(null)
  const sessionStatus = $derived(sessionLoad.status)
  let notes = $state<SessionNote[]>([])
  const notesStatus = $derived(notesLoad.status)
  let active = $state<SessionNote | null>(null)
  let creating = $state(false)
  let editing = $state(false)
  let saving = $state(false)
  let deleting = $state(false)
  let removal = $state<SessionNote | null>(null)
  let collectionOwner = ""
  let historyOwner = ""
  let title = $state("")
  let description = $state("")
  let body = $state("")
  let sensitive = $state(false)
  let error = $state("")
  let history = $state<Revision[]>([])
  let historyLoading = $state(false)
  let historyRequest: AbortController | undefined
  let historyError = $state("")
  let selectedRevision = $state<Revision | null>(null)
  let generation = 0
  let historyGeneration = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as NotesRoute)
  const detailStatus = $derived(
    currentRoute.kind === "session-note-revision"
      ? revisionLoad.status
      : detailLoad.status,
  )
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const workspacePath = (workspaceID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}`
  function updateSessionName(name: string): void {
    if (session !== null) session = { ...session, name }
  }
  const sessionsPath = (workspaceID: string) =>
    `${workspacePath(workspaceID)}/ses`
  const sessionPath = (workspaceID: string, sessionID: string) =>
    `${sessionsPath(workspaceID)}/${encodeURIComponent(sessionID)}`
  const listPath = (workspaceID: string, sessionID: string) =>
    `${sessionPath(workspaceID, sessionID)}/notes`
  const detailPath = (workspaceID: string, sessionID: string, noteID: string) =>
    `${listPath(workspaceID, sessionID)}/${encodeURIComponent(noteID)}`
  const editPath = (workspaceID: string, sessionID: string, noteID: string) =>
    `${detailPath(workspaceID, sessionID, noteID)}/edit`
  const revisionPath = (
    workspaceID: string,
    sessionID: string,
    noteID: string,
    revision: number,
  ) =>
    `${detailPath(workspaceID, sessionID, noteID)}/revisions/${encodeURIComponent(String(revision))}`
  const projectPath = (workspaceID: string, projectID: string) =>
    `${workspacePath(workspaceID)}/prj/${encodeURIComponent(projectID)}`
  const dateLabel = (value: string) => {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const number = (part: number) => part.toString().padStart(2, "0")
    return `${date.getFullYear()}-${number(date.getMonth() + 1)}-${number(date.getDate())} ${number(date.getHours())}:${number(date.getMinutes())}`
  }
  const authorLabel = userAuthorLabel
  const sortNotes = (loaded: SessionNote[]) =>
    [...loaded].sort((left, right) => {
      const difference =
        new Date(right.created_at).getTime() -
        new Date(left.created_at).getTime()
      return Number.isFinite(difference) && difference !== 0
        ? difference
        : right.id.localeCompare(left.id)
    })
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

  function activate(route: NotesRoute): () => void {
    const owner = JSON.stringify([route.workspaceID, route.sessionID, auth.state.claims?.principal.ref.id])
    if (owner !== collectionOwner) {
      collectionOwner = owner
      page.reset()
      sessionLoad.reset()
      notesLoad.reset()
      session = null
      notes = []
      historyOwner = ""
    } else page.suspend()
    detailLoad.reset()
    revisionLoad.reset()
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    historyGeneration += 1
    active = null
    creating = route.kind === "session-note-new"
    editing = creating
    saving = false
    deleting = false
    removal = null
    title = ""
    description = ""
    body = ""
    sensitive = false
    error = ""
    if ("noteID" in route && historyOwner !== route.noteID) {
      historyOwner = route.noteID
      historyPage.reset()
      history = []
    } else historyPage.suspend()
    historyLoading = false
    historyError = ""
    selectedRevision = null
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
        historyPage.suspend()
        sessionLoad.stop()
        notesLoad.stop()
        detailLoad.stop()
        revisionLoad.stop()
        historyRequest?.abort()
        unsubscribe?.()
        unsubscribe = undefined
        historyGeneration += 1
      }
    }
  }

  async function loadRoute(
    route: NotesRoute,
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
    if (!isCurrent(value, workspaceID, sessionID, signal)) return
    subscribe(route, value, signal)
    await loadContent(route, value, signal)
  }

  async function loadSession(
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    const request = sessionLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
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
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
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

  async function loadContent(
    route: NotesRoute,
    value: number,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    const { workspaceID, sessionID } = route
    if (route.kind === "session-notes")
      return loadNotes(value, workspaceID, sessionID, signal, showLoading)
    if (route.kind === "session-note-new") return true
    const loaded = await loadDetail(
      route.noteID,
      value,
      workspaceID,
      sessionID,
      signal,
    )
    if (!loaded || !isCurrent(value, workspaceID, sessionID, signal))
      return false
    if (route.kind === "session-note-edit") {
      if (!editing) startEdit()
      return true
    }
    if (route.kind === "session-note-history") {
      await openHistory(false)
      return historyError === ""
    }
    return (
      route.kind !== "session-note-revision" ||
      (await loadRevision(
        route.noteID,
        route.revision,
        value,
        workspaceID,
        sessionID,
        signal,
      ))
    )
  }

  function subscribe(
    route: NotesRoute,
    value: number,
    routeSignal: AbortSignal,
  ): void {
    unsubscribe?.()
    const { workspaceID, sessionID } = route
    unsubscribe = activity.subscribe(
      [
        {
          name: "session-notes",
          topic: `${workspaceID}/${sessionID}`,
          events: ["session.*", "session_note.*"],
        },
      ],
      async ({ signal }) => {
        if (
          !isCurrent(value, workspaceID, sessionID, routeSignal) ||
          signal.aborted
        )
          return
        const refreshed = await Promise.all([
          loadSession(
            value,
            workspaceID,
            sessionID,
            AbortSignal.any([routeSignal, signal]),
            false,
          ),
          loadContent(
            route,
            value,
            AbortSignal.any([routeSignal, signal]),
            false,
          ),
        ])
        if (
          !refreshed.every(Boolean) ||
          signal.aborted ||
          !isCurrent(value, workspaceID, sessionID, routeSignal)
        )
          throw new Error("session notes refresh failed")
      },
    )
    void activity.poll()
  }

  async function loadNotes(
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    const request = notesLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
    try {
      const requested = page.key()
      const response = await fetchSessionNotes(
        workspaceID,
        sessionID,
        signal,
        page.params(),
      )
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("notes unavailable")
      const loaded = (await response.json()) as SessionNote[]
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      if (requested !== page.key()) return false
      return request.commit(() => {
        page.accept(response)
        notes = loaded
      })
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        request.fail(new Error("Notes could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  async function loadDetail(
    noteID: string,
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
  ): Promise<boolean> {
    const request = detailLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
    try {
      const response = await fetchSessionNote(
        workspaceID,
        sessionID,
        noteID,
        signal,
      )
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (response.status === 404) {
        runtime.navigate(listPath(workspaceID, sessionID), true)
        return false
      }
      if (!response.ok) throw new Error("note unavailable")
      const loaded = (await response.json()) as SessionNote
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      return request.commit(() => {
        active = loaded
      })
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        request.fail(new Error("Note could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  async function loadRevision(
    noteID: string,
    revision: number,
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
  ): Promise<boolean> {
    const request = revisionLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
    try {
      const response = await fetchSessionNoteRevision(
        workspaceID,
        sessionID,
        noteID,
        revision,
        signal,
      )
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (response.status === 404) {
        runtime.navigate(detailPath(workspaceID, sessionID, noteID), true)
        return false
      }
      if (!response.ok) throw new Error("revision unavailable")
      const loaded = (await response.json()) as Revision
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      return request.commit(() => {
        selectedRevision = loaded
      })
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        request.fail(new Error("Revision could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  function startEdit(): void {
    if (active === null) return
    title = active.title
    description = active.description
    body = active.body ?? ""
    sensitive = active.sensitive
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
    const note = active
    const input = { title, description, body, sensitive }
    saving = true
    error = ""
    try {
      const response = creating
        ? await createSessionNote(
            route.workspaceID,
            route.sessionID,
            input,
            signal,
          )
        : note === null
          ? undefined
          : await updateSessionNote(
              route.workspaceID,
              route.sessionID,
              note.id,
              input,
              signal,
            )
      if (
        response === undefined ||
        !isCurrent(value, route.workspaceID, route.sessionID, signal)
      )
        return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("note could not be saved")
      const saved = (await response.json()) as SessionNote
      if (!isCurrent(value, route.workspaceID, route.sessionID, signal)) return
      detailLoad.stop()
      notesLoad.stop()
      notes = notes.map((candidate) => candidate.id === saved.id ? saved : candidate)
      runtime.navigate(detailPath(route.workspaceID, route.sessionID, saved.id))
    } catch {
      if (isCurrent(value, route.workspaceID, route.sessionID, signal))
        error = "The note could not be saved. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.sessionID, signal))
        saving = false
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
    const note = removal
    const value = generation
    const signal = abortController?.signal
    if (signal === undefined) return
    deleting = true
    error = ""
    try {
      const response = await removeSessionNote(
        route.workspaceID,
        route.sessionID,
        note.id,
        signal,
      )
      if (
        !isCurrent(value, route.workspaceID, route.sessionID, signal) ||
        active?.id !== note.id
      )
        return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("note could not be removed")
      notes = notes.filter((candidate) => candidate.id !== note.id)
      runtime.navigate(listPath(route.workspaceID, route.sessionID))
    } catch {
      if (isCurrent(value, route.workspaceID, route.sessionID, signal))
        error = "The note could not be removed. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.sessionID, signal))
        deleting = false
    }
  }

  async function openHistory(reset = true): Promise<void> {
    if (active === null) return
    if (reset) historyPage.reset()
    const route = currentRoute
    const note = active
    const value = generation
    const signal = abortController?.signal
    if (signal === undefined) return
    const valueForHistory = ++historyGeneration
    historyRequest?.abort()
    const request = new AbortController()
    historyRequest = request
    if (reset) history = []
    historyError = ""
    historyLoading = true
    try {
      const response = await fetchSessionNoteRevisions(
        route.workspaceID,
        route.sessionID,
        note.id,
        AbortSignal.any([signal, request.signal]),
        historyPage.params(),
      )
      if (
        !isCurrent(value, route.workspaceID, route.sessionID, signal) ||
        active?.id !== note.id ||
        valueForHistory !== historyGeneration
      )
        return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("history unavailable")
      const loaded = (await response.json()) as Revision[]
      if (
        !isCurrent(value, route.workspaceID, route.sessionID, signal) ||
        active?.id !== note.id ||
        valueForHistory !== historyGeneration
      )
        return
      historyPage.accept(response)
      history = [...loaded].sort(
        (left, right) => right.revision - left.revision,
      )
    } catch {
      if (
        isCurrent(value, route.workspaceID, route.sessionID, signal) &&
        active?.id === note.id &&
        valueForHistory === historyGeneration
      )
        historyError = "The note history could not be loaded. Try again."
    } finally {
      if (
        isCurrent(value, route.workspaceID, route.sessionID, signal) &&
        valueForHistory === historyGeneration
      )
        historyLoading = false
    }
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
      ><UI.RouterLink
        variant="default"
        href={runtime.loginHref} replace>Sign in</UI.RouterLink
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
      ><UI.RouterLink
        variant="default"
        href="/app/no-access" replace
        >Continue</UI.RouterLink
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
              {#if currentRoute.kind === "session-notes"}
                <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                  aria-current="page"
                  ><UI.Breadcrumb.Page>Notes</UI.Breadcrumb.Page
                  ></UI.Breadcrumb.Item
                >
              {:else}
                <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                  ><RouterLink href={listPath(workspace.id, session.id)}
                    >Notes</RouterLink
                  ></UI.Breadcrumb.Item
                >
                <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                  aria-current="page"
                  ><UI.Breadcrumb.Page
                    >{currentRoute.kind === "session-note-new"
                      ? "New Note"
                      : selectedRevision === null
                        ? (active?.title ?? "Note")
                        : `Revision ${selectedRevision.revision}`}</UI.Breadcrumb.Page
                  ></UI.Breadcrumb.Item
                >
              {/if}
            {/if}
          </UI.Breadcrumb.List>
        </UI.Breadcrumb.Root>
        {#if session !== null}<SessionViewNavigation
            workspaceID={workspace.id}
            sessionID={session.id}
            items={sessionViews(workspace.id, session.id, "notes")}
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
    <PageBody fluid>
          {#if sessionLoad.reloading || notesLoad.reloading || detailLoad.reloading || revisionLoad.reloading}<UI.Text
              role="status">Refreshing notes…</UI.Text
            >{/if}
          {#if sessionLoad.state.error || notesLoad.state.error || detailLoad.state.error || revisionLoad.state.error}<UI.Notice
              variant="destructive"
              role="alert"
              >{sessionLoad.state.error ||
                notesLoad.state.error ||
                detailLoad.state.error ||
                revisionLoad.state.error}<UI.Button
                variant="outline"
                onclick={() =>
                  void loadRoute(
                    currentRoute,
                    generation,
                    abortController!.signal,
                  )}>Retry</UI.Button
              ></UI.Notice
            >{/if}
          {#if sessionStatus === "checking"}<UI.Text
              as="p"
              tone="muted"
              aria-busy="true"
              aria-live="polite"
            >
              Loading chat...
            </UI.Text>
          {:else if sessionStatus === "unavailable"}<UI.Text
              as="p"
              tone="muted"
            >
              This chat could not be loaded.
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
          {:else if removal !== null}
            <UI.ResourceRemoval noun="note" name={removal.title} busy={deleting} {error} onCancel={() => { removal = null; error = "" }} onConfirm={() => void remove()} />
          {:else if editing}<UI.AdminHeader
              compact={embedded}
              title={creating ? "New note" : `Edit ${active?.title ?? "note"}`}
              description={embedded ? undefined : "Update the summary, content, and sensitivity."}
            /><UI.Form
              id="session-note-form"
              surface={!embedded}
              compact={embedded}
              onsubmit={(event) => {
                event.preventDefault()
                void save()
              }}
            >
              <UI.FormSection
                dialog={embedded}
                title="Summary"
                description="A title and short description identify this note."
                ><UI.Field>
                  <UI.Label for="session-note-title">Title</UI.Label>
                  <UI.Input
                    id="session-note-title"
                    autocomplete="off"
                    maxlength={256}
                    required
                    bind:value={title}
                  />
                </UI.Field>
                <UI.Field>
                  <UI.Label for="session-note-description"
                    >Description (optional)</UI.Label
                  >
                  <UI.Textarea
                    id="session-note-description"
                    autocomplete="off"
                    rows={3}
                    maxlength={4096}
                    bind:value={description}
                  ></UI.Textarea>
                </UI.Field>
              </UI.FormSection><UI.FormSection
                dialog={embedded}
                title="Content"
                description="Write the note using Markdown."
                ><UI.Field>
                  <UI.Label for="session-note-body">Content (optional Markdown)</UI.Label
                  >
                  <UI.Textarea
                    id="session-note-body"
                    autocomplete="off"
                    rows={18}
                    maxlength={1048576}
                    bind:value={body}
                  ></UI.Textarea>
                </UI.Field>
              </UI.FormSection><UI.FormSection
                dialog={embedded}
                title="Sensitivity"
                description="Control how agents handle the note content."
                ><UI.Field>
                  <UI.Label layout="choice"
                    ><UI.Checkbox bind:checked={sensitive} /> Sensitive: content
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
              <UI.FormActions dialog={embedded}>
                <UI.RouterLink
                  variant="outline"
                  disabled={saving}
                  href={creating || active === null
                    ? listPath(currentRoute.workspaceID, currentRoute.sessionID)
                    : detailPath(currentRoute.workspaceID, currentRoute.sessionID, active.id)}>Cancel</UI.RouterLink
                ><UI.Button variant="default" type="submit" form="session-note-form" disabled={saving}
                  >{saving ? "Saving..." : "Save note"}</UI.Button
                >
              </UI.FormActions>
            </UI.Form>
          {:else if currentRoute.kind === "session-note-history" && (history.length > 0 || detailStatus === "ready")}
            {@render historyContent()}
          {:else if "noteID" in currentRoute && detailStatus === "checking"}<UI.Text
              as="p"
              tone="muted"
            >
              Loading note...
            </UI.Text>
          {:else if "noteID" in currentRoute && detailStatus === "unavailable"}<UI.Text
              as="p"
              tone="muted"
            >
              Note unavailable.
            </UI.Text>
            <UI.Button
              variant="default"
              type="button"
              onclick={() => {
                const signal = abortController?.signal
                if (signal !== undefined)
                  void loadContent(currentRoute, generation, signal)
              }}>Try again</UI.Button
            >
          {:else if active !== null}{@const noteID = active.id}{@const displayed =
              selectedRevision ?? active}
            {#snippet noteActions()}
              <UI.RouterLink variant="outline" href={`${detailPath(currentRoute.workspaceID, currentRoute.sessionID, noteID)}/revisions`}>History</UI.RouterLink>
              {#if selectedRevision === null}
                <UI.Button variant="outline" disabled={deleting} onclick={requestRemove}>Remove</UI.Button>
                <UI.RouterLink variant={embedded ? "default" : "outline"} href={editPath(currentRoute.workspaceID, currentRoute.sessionID, noteID)}>Edit</UI.RouterLink>
              {:else}
                <UI.RouterLink variant={embedded ? "default" : "outline"} href={detailPath(currentRoute.workspaceID, currentRoute.sessionID, noteID)}>Current revision</UI.RouterLink>
              {/if}
            {/snippet}
            <UI.Stack as="article">
              <UI.AdminHeader
                compact={embedded}
                kind={selectedRevision === null
                  ? embedded ? undefined : "Chat note"
                  : `Historical revision ${selectedRevision.revision}`}
                title={displayed.title}
                description={displayed.description}
                actions={embedded ? undefined : noteActions}
              />
              {#if embedded}<UI.ResourceActions>{@render noteActions()}</UI.ResourceActions>{/if}
              <UI.DetailSection title="Note information" compact={embedded}>
                <UI.Property label="Author"
                  >{authorLabel(displayed.author)}</UI.Property
                >
                <UI.Property label="Date"
                  >{dateLabel(displayed.created_at)}</UI.Property
                >
                <UI.Property label="Revision">{displayed.revision}</UI.Property>
                <UI.Property label="Sensitivity"
                  >{displayed.sensitive ? "Sensitive" : "Standard"}</UI.Property
                >
              </UI.DetailSection>
              {#if displayed.body !== undefined && displayed.body !== ""}<UI.RichText
                  html={renderMarkdown(displayed.body)}
                ></UI.RichText>{/if}{#if error !== ""}<UI.Text
                  as="p"
                  size="small"
                  role="alert"
                  aria-live="polite"
                >
                  {error}
                </UI.Text>{/if}
            </UI.Stack>
          {:else}{#snippet collectionActions()}<UI.RouterLink
                variant="default"
                href={`${listPath(workspace.id, currentRoute.sessionID)}/new`}>New note</UI.RouterLink
              >{/snippet}
            {#if embedded}
              <UI.ResourceActions>{@render collectionActions()}</UI.ResourceActions>
            {:else}
              <UI.AdminHeader
                title="Notes"
                description="Documents associated with this chat."
                actions={collectionActions}
              />
            {/if}
            <UI.RecordList
              label="Notes"
              items={notes.map((note) => ({ ...note, label: note.title }))}
              href={(note) => detailPath(workspace.id, currentRoute.sessionID, note.id)}
              loading={page.state.busy || notesLoad.state.pending}
              scrollable={false}
              pageKey={JSON.stringify([page.state.query, page.state.cursor])}
              emptyMessage={notesStatus === "checking" ? "Loading…" : notesStatus === "unavailable" ? "Notes could not be loaded." : "No matching notes."}
            >
              {#snippet toolbar()}<UI.SearchToolbar framed={false} label="Search notes" placeholder="Search" bind:value={page.state.search} loading={page.state.busy || notesLoad.state.pending} onsearch={() => void page.search(reloadNotes)} />{/snippet}
              {#snippet badges(note)}{#if note.sensitive}<UI.Badge>Sensitive</UI.Badge>{/if}{/snippet}
              {#snippet details(note)}{note.description || `${authorLabel(note.author)} · ${dateLabel(note.created_at)}`}{/snippet}
            </UI.RecordList>
            {#if page.state.history.length > 0 || page.state.next}<UI.PaginationFooter
              showCount={false}
              count={notes.length}
              previous={page.state.history.length > 0}
              next={!!page.state.next}
              loading={page.state.busy || notesLoad.state.pending}
              onprevious={() => void page.previous(reloadNotes)}
              onnext={() => void page.next(reloadNotes)}
            />{/if}
            {#if notesStatus === "unavailable"}<UI.Button
                variant="outline"
                onclick={() =>
                  void loadNotes(
                    generation,
                    currentRoute.workspaceID,
                    currentRoute.sessionID,
                    abortController!.signal,
                  )}>Try again</UI.Button
              >{/if}
          {/if}
    </PageBody>
  </UI.ResourcePage>
  {#snippet historyContent()}
    <UI.AdminHeader title="Revision history" compact={embedded} />
    {#if historyError !== ""}<UI.Notice
          variant="destructive"
          role="alert"
        >
          {historyError}
          <UI.Button variant="outline" onclick={() => void reloadHistory()}>Try again</UI.Button>
        </UI.Notice>{/if}
      <UI.RecordList
        label="Revisions"
        items={history.map((revision) => ({ ...revision, id: String(revision.revision), label: revision.title }))}
        href={(revision) => revision.revision === active?.revision ? detailPath(currentRoute.workspaceID, currentRoute.sessionID, active.id) : revisionPath(currentRoute.workspaceID, currentRoute.sessionID, "noteID" in currentRoute ? currentRoute.noteID : "", revision.revision)}
        scrollable={false}
        loading={historyLoading || historyPage.state.busy}
        pageKey={JSON.stringify([historyPage.state.query, historyPage.state.cursor])}
        emptyMessage={historyLoading ? "Loading…" : "No matching revisions."}
      >
        {#snippet toolbar()}<UI.SearchToolbar framed={false} label="Search revisions" placeholder="Search" bind:value={historyPage.state.search} loading={historyLoading || historyPage.state.busy} onsearch={() => void historyPage.search(reloadHistory)} />{/snippet}
        {#snippet badges(revision)}
          <UI.Badge>Revision {revision.revision}</UI.Badge>
          {#if revision.revision === active?.revision}<UI.Badge>Current</UI.Badge>{/if}
          {#if revision.sensitive}<UI.Badge>Sensitive</UI.Badge>{/if}
        {/snippet}
        {#snippet details(revision)}{authorLabel(revision.author)} · {dateLabel(revision.created_at)}{/snippet}
      </UI.RecordList>
      {#if historyPage.state.history.length > 0 || historyPage.state.next}<UI.PaginationFooter
        showCount={false}
        count={history.length}
        previous={historyPage.state.history.length > 0}
        next={!!historyPage.state.next}
        loading={historyLoading || historyPage.state.busy}
        onprevious={() => void historyPage.previous(reloadHistory)}
        onnext={() => void historyPage.next(reloadHistory)}
      />{/if}
  {/snippet}
{/if}
