<script lang="ts">
  import {
    X,
    Modal,
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

  const runtime = useRuntime()
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
  let sessionStatus = $state<Status>("checking")
  let notes = $state<SessionNote[]>([])
  let notesStatus = $state<Status>("checking")
  let active = $state<SessionNote | null>(null)
  let detailStatus = $state<Status>("ready")
  let creating = $state(false)
  let editing = $state(false)
  let saving = $state(false)
  let deleting = $state(false)
  let title = $state("")
  let description = $state("")
  let body = $state("")
  let sensitive = $state(false)
  let error = $state("")
  let historyModal = $state<UI.DialogHandle | undefined>()
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

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })

  function activate(route: NotesRoute): () => void {
    page.reset()
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    historyGeneration += 1
    if (historyModal?.open) historyModal.close()
    session = null
    sessionStatus = "checking"
    notes = []
    notesStatus = "checking"
    active = null
    detailStatus =
      route.kind === "session-note" ||
      route.kind === "session-note-edit" ||
      route.kind === "session-note-revision"
        ? "checking"
        : "ready"
    creating = route.kind === "session-note-new"
    editing = creating
    saving = false
    deleting = false
    title = ""
    description = ""
    body = ""
    sensitive = false
    error = ""
    history = []
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
    if (showLoading && isCurrent(value, workspaceID, sessionID, signal))
      sessionStatus = "checking"
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
      session = loaded
      sessionStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        sessionStatus = "unavailable"
      return false
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
          loadSession(value, workspaceID, sessionID, routeSignal, false),
          loadContent(route, value, routeSignal, false),
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
    if (showLoading && isCurrent(value, workspaceID, sessionID, signal))
      notesStatus = "checking"
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
      page.accept(response)
      notes = loaded
      notesStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        notesStatus = "unavailable"
      return false
    }
  }

  async function loadDetail(
    noteID: string,
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
  ): Promise<boolean> {
    if (isCurrent(value, workspaceID, sessionID, signal))
      detailStatus = "checking"
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
      active = loaded
      detailStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        detailStatus = "unavailable"
      return false
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
    if (isCurrent(value, workspaceID, sessionID, signal))
      detailStatus = "checking"
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
      selectedRevision = loaded
      detailStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        detailStatus = "unavailable"
      return false
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

  function cancelEdit(): void {
    if (saving) return
    error = ""
    runtime.navigate(
      creating
        ? listPath(currentRoute.workspaceID, currentRoute.sessionID)
        : active === null
          ? listPath(currentRoute.workspaceID, currentRoute.sessionID)
          : detailPath(
              currentRoute.workspaceID,
              currentRoute.sessionID,
              active.id,
            ),
    )
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
      notes = sortNotes([
        saved,
        ...notes.filter((candidate) => candidate.id !== saved.id),
      ])
      notesStatus = "ready"
      runtime.navigate(detailPath(route.workspaceID, route.sessionID, saved.id))
    } catch {
      if (isCurrent(value, route.workspaceID, route.sessionID, signal))
        error = "The note could not be saved. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.sessionID, signal))
        saving = false
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
    const note = active
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
    history = []
    historyError = ""
    historyLoading = true
    if (!historyModal?.open) historyModal?.showModal()
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

  function selectRevision(revision: number): void {
    if (active === null) return
    const route = currentRoute
    historyModal?.close()
    runtime.navigate(
      revision === active.revision
        ? detailPath(route.workspaceID, route.sessionID, active.id)
        : revisionPath(route.workspaceID, route.sessionID, active.id, revision),
    )
  }

  function closeHistory(): void {
    historyRequest?.abort()
    historyLoading = false
    historyPage.reset()
    historyGeneration += 1
    historyError = ""
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
        {#if session !== null}<UI.ViewNavigation
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
          />{/if}</SidebarPage.Header
      >
      <SidebarPage.Body
        ><PageBody fluid>
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
          {:else if editing}<UI.AdminHeader
              title={creating ? "New note" : `Edit ${active?.title ?? "note"}`}
              description="Update the summary, content, and sensitivity."
            /><UI.Form
              surface
              onsubmit={(event) => {
                event.preventDefault()
                void save()
              }}
            >
              <UI.FormSection
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
                title="Content"
                description="Write the note using Markdown."
                ><UI.Field>
                  <UI.Label for="session-note-body">Content (optional)</UI.Label
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
              <UI.FormActions>
                <UI.Button
                  variant="outline"
                  type="button"
                  disabled={saving}
                  onclick={cancelEdit}>Cancel</UI.Button
                ><UI.Button variant="default" type="submit" disabled={saving}
                  >{saving ? "Saving..." : "Save note"}</UI.Button
                >
              </UI.FormActions>
            </UI.Form>
          {:else if (currentRoute.kind === "session-note" || currentRoute.kind === "session-note-edit" || currentRoute.kind === "session-note-revision") && detailStatus === "checking"}<UI.Text
              as="p"
              tone="muted"
            >
              Loading note...
            </UI.Text>
          {:else if (currentRoute.kind === "session-note" || currentRoute.kind === "session-note-edit" || currentRoute.kind === "session-note-revision") && detailStatus === "unavailable"}<UI.Text
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
          {:else if active !== null}{@const displayed =
              selectedRevision ?? active}
            {#snippet noteActions()}{#if selectedRevision === null}<UI.Button
                  variant="outline"
                  size="sm"
                  type="button"
                  onclick={() =>
                    active !== null &&
                    runtime.navigate(
                      editPath(
                        currentRoute.workspaceID,
                        currentRoute.sessionID,
                        active.id,
                      ),
                    )}>Edit</UI.Button
                >{:else}<UI.Button
                  variant="outline"
                  size="sm"
                  type="button"
                  onclick={() =>
                    active !== null &&
                    runtime.navigate(
                      detailPath(
                        currentRoute.workspaceID,
                        currentRoute.sessionID,
                        active.id,
                      ),
                    )}>Current revision</UI.Button
                >{/if}<UI.Button
                variant="outline"
                size="sm"
                type="button"
                onclick={() => void openHistory()}>History</UI.Button
              >{#if selectedRevision === null}<UI.Button
                  variant="outline"
                  size="sm"
                  type="button"
                  disabled={deleting}
                  onclick={() => void remove()}
                  >{deleting ? "Removing..." : "Remove"}</UI.Button
                >{/if}{/snippet}
            <UI.Stack as="article">
              <UI.AdminHeader
                kind={selectedRevision === null
                  ? "Chat note"
                  : `Historical revision ${selectedRevision.revision}`}
                title={displayed.title}
                description={displayed.description}
                actions={noteActions}
              />
              <UI.DetailSection title="Note information">
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
          {:else}{#snippet collectionActions()}<UI.Button
                variant="default"
                size="sm"
                type="button"
                onclick={() =>
                  runtime.navigate(
                    `${listPath(workspace.id, currentRoute.sessionID)}/new`,
                  )}>New note</UI.Button
              >{/snippet}
            <UI.AdminHeader
              title="Notes"
              description="Documents associated with this chat."
              actions={collectionActions}
            />
            <UI.SearchToolbar
              label="Search notes"
              placeholder="Title or description"
              bind:value={page.state.search}
              loading={page.state.busy || notesStatus === "checking"}
              onsearch={() => void page.search(reloadNotes)}
            />
            <UI.RecordTable
              headers={["Title", "Sensitivity", "Author", "Date"]}
            >
              {#if notesStatus !== "ready"}<UI.TableEmpty
                  columns={4}
                  message={notesStatus === "checking"
                    ? "Loading notes…"
                    : "Notes could not be loaded."}
                />
              {:else}{#each notes as note (note.id)}<UI.RecordRow>
                    <UI.RecordCell primary
                      ><RouterLink
                        href={detailPath(
                          workspace.id,
                          currentRoute.sessionID,
                          note.id,
                        )}>{note.title}</RouterLink
                      ><UI.Text size="small" tone="muted"
                        >{note.description}</UI.Text
                      ></UI.RecordCell
                    >
                    <UI.RecordCell
                      >{note.sensitive
                        ? "Sensitive"
                        : "Standard"}</UI.RecordCell
                    >
                    <UI.RecordCell>{authorLabel(note.author)}</UI.RecordCell>
                    <UI.RecordCell>{dateLabel(note.created_at)}</UI.RecordCell>
                  </UI.RecordRow>{:else}<UI.TableEmpty
                    columns={4}
                    message="No notes yet."
                  />{/each}{/if}
            </UI.RecordTable>
            <UI.PaginationFooter
              noun="notes"
              count={notes.length}
              previous={page.state.history.length > 0}
              next={!!page.state.next}
              loading={page.state.busy || notesStatus === "checking"}
              onprevious={() => void page.previous(reloadNotes)}
              onnext={() => void page.next(reloadNotes)}
            />
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
        </PageBody></SidebarPage.Body
      >
    </SidebarPage.Page>
  </SidebarPage.Root>
  <Modal
    title="Revision history"
    bind:modal={historyModal}
    labelledBy="session-note-history-heading"
    onClose={closeHistory}
  >
    {#snippet header()}<UI.Heading level={2} id="session-note-history-heading"
        >Revision history</UI.Heading
      >
      <UI.Button
        variant="ghost"
        size="icon"
        type="button"
        aria-label="Close"
        onclick={() => historyModal?.close()}
        ><X size={18} strokeWidth={2} aria-hidden="true" /></UI.Button
      >{/snippet}
    {#if historyLoading}<UI.Text as="p" tone="muted">
        Loading history...
      </UI.Text>{:else}{#if historyError !== ""}<UI.Text
          as="p"
          size="small"
          role="alert"
          aria-live="polite"
        >
          {historyError}
        </UI.Text>{/if}
      <UI.SearchToolbar
        label="Search revisions"
        placeholder="Title or revision"
        bind:value={historyPage.state.search}
        loading={historyLoading || historyPage.state.busy}
        onsearch={() => void historyPage.search(reloadHistory)}
      />
      <UI.RecordTable headers={["Revision", "Title", "Author", "Date"]}>
        {#each history as revision (revision.revision)}<UI.RecordRow>
            <UI.RecordCell primary
              ><UI.Button
                variant="link"
                onclick={() => selectRevision(revision.revision)}
                >Revision {revision.revision}</UI.Button
              >{#if revision.revision === active?.revision}<UI.Badge
                  >Current</UI.Badge
                >{/if}</UI.RecordCell
            >
            <UI.RecordCell
              >{revision.title}{#if revision.sensitive}<UI.Badge
                  >Sensitive</UI.Badge
                >{/if}</UI.RecordCell
            >
            <UI.RecordCell>{authorLabel(revision.author)}</UI.RecordCell>
            <UI.RecordCell>{dateLabel(revision.created_at)}</UI.RecordCell>
          </UI.RecordRow>{:else}<UI.TableEmpty
            columns={4}
            message="No matching revisions."
          />{/each}
      </UI.RecordTable>
      <UI.PaginationFooter
        noun="revisions"
        count={history.length}
        previous={historyPage.state.history.length > 0}
        next={!!historyPage.state.next}
        loading={historyLoading || historyPage.state.busy}
        onprevious={() => void historyPage.previous(reloadHistory)}
        onnext={() => void historyPage.next(reloadHistory)}
      />{/if}
  </Modal>
{/if}
