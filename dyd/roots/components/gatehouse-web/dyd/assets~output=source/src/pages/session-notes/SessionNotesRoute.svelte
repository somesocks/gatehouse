<script lang="ts">
  import { Menu, X } from "@lucide/svelte"
  import { signOut } from "../../app/auth"
  import { fetchChatSession } from "../../app/chat"
  import { useRuntime } from "../../app/runtime.svelte"
  import { createSessionNote, fetchSessionNote, fetchSessionNoteRevision, fetchSessionNoteRevisions, fetchSessionNotes, removeSessionNote, updateSessionNote, type SessionNote } from "../../app/session-notes"
  import ModalDialog from "../../components/ModalDialog.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import SessionNavigation from "../../components/SessionNavigation.svelte"
  import WorkspaceFrame from "../../components/WorkspaceFrame.svelte"
  import { renderMarkdown } from "../../markdown"
  import type { Route } from "../../route"

  type NotesRoute = Extract<Route, { kind: "session-notes" | "session-note-new" | "session-note" | "session-note-edit" | "session-note-revision" }>
  type Session = { id: string; created_at: string; name?: string; project?: { id: string; name?: string } }
  type Status = "checking" | "ready" | "unavailable"
  type Revision = Pick<SessionNote, "title" | "description" | "sensitive" | "author" | "created_at"> & { revision: number; body?: string }

  const runtime = useRuntime()
  const { access, activity, auth } = runtime
  let mobileMenuOpen = $state(false)
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
  let historyDialog = $state<HTMLDialogElement | undefined>()
  let history = $state<Revision[]>([])
  let historyLoading = $state(false)
  let historyError = $state("")
  let selectedRevision = $state<Revision | null>(null)
  let generation = 0
  let historyGeneration = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as NotesRoute)
  const workspace = $derived(access.state.workspaces.find((candidate) => candidate.id === currentRoute.workspaceID) ?? null)
  const workspacePath = (workspaceID: string) => `/app/wsp/${encodeURIComponent(workspaceID)}`
  const sessionsPath = (workspaceID: string) => `${workspacePath(workspaceID)}/ses`
  const sessionPath = (workspaceID: string, sessionID: string) => `${sessionsPath(workspaceID)}/${encodeURIComponent(sessionID)}`
  const listPath = (workspaceID: string, sessionID: string) => `${sessionPath(workspaceID, sessionID)}/notes`
  const detailPath = (workspaceID: string, sessionID: string, noteID: string) => `${listPath(workspaceID, sessionID)}/${encodeURIComponent(noteID)}`
  const editPath = (workspaceID: string, sessionID: string, noteID: string) => `${detailPath(workspaceID, sessionID, noteID)}/edit`
  const revisionPath = (workspaceID: string, sessionID: string, noteID: string, revision: number) => `${detailPath(workspaceID, sessionID, noteID)}/revisions/${encodeURIComponent(String(revision))}`
  const projectPath = (workspaceID: string, projectID: string) => `${workspacePath(workspaceID)}/prj/${encodeURIComponent(projectID)}`
  const dateLabel = (value: string) => {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return value
    const number = (part: number) => part.toString().padStart(2, "0")
    return `${date.getFullYear()}-${number(date.getMonth() + 1)}-${number(date.getDate())} ${number(date.getHours())}:${number(date.getMinutes())}`
  }
  const authorLabel = (author: SessionNote["author"]) => author.principal?.name ?? author.principal?.id ?? author.agent?.label ?? author.agent?.id ?? author.gateway ?? "Unknown"
  const sortNotes = (loaded: SessionNote[]) => [...loaded].sort((left, right) => {
    const difference = new Date(right.created_at).getTime() - new Date(left.created_at).getTime()
    return Number.isFinite(difference) && difference !== 0 ? difference : right.id.localeCompare(left.id)
  })
  const isCurrent = (value: number, workspaceID: string, sessionID: string, signal: AbortSignal) => value === generation && !signal.aborted && currentRoute.workspaceID === workspaceID && currentRoute.sessionID === sessionID

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })

  function activate(route: NotesRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    historyGeneration += 1
    if (historyDialog?.open) historyDialog.close()
    mobileMenuOpen = false
    session = null
    sessionStatus = "checking"
    notes = []
    notesStatus = "checking"
    active = null
    detailStatus = route.kind === "session-note" || route.kind === "session-note-edit" || route.kind === "session-note-revision" ? "checking" : "ready"
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
    if (auth.state.status === "authenticated" && access.state.workspaceStatus === "ready") void loadRoute(route, value, abortController.signal)
    else if (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking") void runtime.refresh()
    return () => {
      if (value === generation) {
        abortController?.abort()
        unsubscribe?.()
        unsubscribe = undefined
        historyGeneration += 1
      }
    }
  }

  async function loadRoute(route: NotesRoute, value: number, signal: AbortSignal): Promise<void> {
    const { workspaceID, sessionID } = route
    if (!access.state.workspaces.some((candidate) => candidate.id === workspaceID)) {
      if (isCurrent(value, workspaceID, sessionID, signal)) runtime.navigate(access.state.workspaces.length === 0 ? "/app/no-access" : workspacePath(access.state.workspaces[0].id), true)
      return
    }
    if (!await loadSession(value, workspaceID, sessionID, signal)) return
    if (!isCurrent(value, workspaceID, sessionID, signal)) return
    subscribe(route, value, signal)
    await loadContent(route, value, signal)
  }

  async function loadSession(value: number, workspaceID: string, sessionID: string, signal: AbortSignal, showLoading = true): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, sessionID, signal)) sessionStatus = "checking"
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
      const loaded = await response.json() as Session
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      session = loaded
      sessionStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal)) sessionStatus = "unavailable"
      return false
    }
  }

  async function loadContent(route: NotesRoute, value: number, signal: AbortSignal, showLoading = true): Promise<boolean> {
    const { workspaceID, sessionID } = route
    const listed = await loadNotes(value, workspaceID, sessionID, signal, showLoading)
    if (!listed || !isCurrent(value, workspaceID, sessionID, signal)) return false
    if (route.kind === "session-notes" || route.kind === "session-note-new") return true
    const loaded = await loadDetail(route.noteID, value, workspaceID, sessionID, signal)
    if (!loaded || !isCurrent(value, workspaceID, sessionID, signal)) return false
    if (route.kind === "session-note-edit") {
      if (!editing) startEdit()
      return true
    }
    return route.kind !== "session-note-revision" || await loadRevision(route.noteID, route.revision, value, workspaceID, sessionID, signal)
  }

  function subscribe(route: NotesRoute, value: number, routeSignal: AbortSignal): void {
    const { workspaceID, sessionID } = route
    unsubscribe = activity.subscribe([{ name: "session-notes", topic: `${workspaceID}/${sessionID}`, events: ["session.*", "session_note.*"] }], async ({ signal }) => {
      if (!isCurrent(value, workspaceID, sessionID, routeSignal) || signal.aborted) return
      const refreshed = await Promise.all([loadSession(value, workspaceID, sessionID, routeSignal, false), loadContent(route, value, routeSignal, false)])
      if (!refreshed.every(Boolean) || signal.aborted || !isCurrent(value, workspaceID, sessionID, routeSignal)) throw new Error("session notes refresh failed")
    })
    void activity.poll()
  }

  async function loadNotes(value: number, workspaceID: string, sessionID: string, signal: AbortSignal, showLoading = true): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, sessionID, signal)) notesStatus = "checking"
    try {
      const response = await fetchSessionNotes(workspaceID, sessionID, signal)
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error("notes unavailable")
      const loaded = await response.json() as SessionNote[]
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      notes = sortNotes(loaded)
      notesStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal)) notesStatus = "unavailable"
      return false
    }
  }

  async function loadDetail(noteID: string, value: number, workspaceID: string, sessionID: string, signal: AbortSignal): Promise<boolean> {
    if (isCurrent(value, workspaceID, sessionID, signal)) detailStatus = "checking"
    try {
      const response = await fetchSessionNote(workspaceID, sessionID, noteID, signal)
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
      const loaded = await response.json() as SessionNote
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      active = loaded
      detailStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal)) detailStatus = "unavailable"
      return false
    }
  }

  async function loadRevision(noteID: string, revision: number, value: number, workspaceID: string, sessionID: string, signal: AbortSignal): Promise<boolean> {
    if (isCurrent(value, workspaceID, sessionID, signal)) detailStatus = "checking"
    try {
      const response = await fetchSessionNoteRevision(workspaceID, sessionID, noteID, revision, signal)
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
      const loaded = await response.json() as Revision
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      selectedRevision = loaded
      detailStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal)) detailStatus = "unavailable"
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
    runtime.navigate(creating ? listPath(currentRoute.workspaceID, currentRoute.sessionID) : active === null ? listPath(currentRoute.workspaceID, currentRoute.sessionID) : detailPath(currentRoute.workspaceID, currentRoute.sessionID, active.id))
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
      const response = creating ? await createSessionNote(route.workspaceID, route.sessionID, input, signal) : note === null ? undefined : await updateSessionNote(route.workspaceID, route.sessionID, note.id, input, signal)
      if (response === undefined || !isCurrent(value, route.workspaceID, route.sessionID, signal)) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("note could not be saved")
      const saved = await response.json() as SessionNote
      if (!isCurrent(value, route.workspaceID, route.sessionID, signal)) return
      notes = sortNotes([saved, ...notes.filter((candidate) => candidate.id !== saved.id)])
      notesStatus = "ready"
      runtime.navigate(detailPath(route.workspaceID, route.sessionID, saved.id))
    } catch {
      if (isCurrent(value, route.workspaceID, route.sessionID, signal)) error = "The note could not be saved. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.sessionID, signal)) saving = false
    }
  }

  async function remove(): Promise<void> {
    if (active === null || deleting || !window.confirm(`Remove ${active.title}?`)) return
    const route = currentRoute
    const note = active
    const value = generation
    const signal = abortController?.signal
    if (signal === undefined) return
    deleting = true
    error = ""
    try {
      const response = await removeSessionNote(route.workspaceID, route.sessionID, note.id, signal)
      if (!isCurrent(value, route.workspaceID, route.sessionID, signal) || active?.id !== note.id) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("note could not be removed")
      notes = notes.filter((candidate) => candidate.id !== note.id)
      runtime.navigate(listPath(route.workspaceID, route.sessionID))
    } catch {
      if (isCurrent(value, route.workspaceID, route.sessionID, signal)) error = "The note could not be removed. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.sessionID, signal)) deleting = false
    }
  }

  async function openHistory(): Promise<void> {
    if (active === null) return
    const route = currentRoute
    const note = active
    const value = generation
    const signal = abortController?.signal
    if (signal === undefined) return
    const valueForHistory = ++historyGeneration
    history = []
    historyError = ""
    historyLoading = true
    if (!historyDialog?.open) historyDialog?.showModal()
    try {
      const response = await fetchSessionNoteRevisions(route.workspaceID, route.sessionID, note.id, signal)
      if (!isCurrent(value, route.workspaceID, route.sessionID, signal) || active?.id !== note.id || valueForHistory !== historyGeneration) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error("history unavailable")
      const loaded = await response.json() as Revision[]
      if (!isCurrent(value, route.workspaceID, route.sessionID, signal) || active?.id !== note.id || valueForHistory !== historyGeneration) return
      history = [...loaded].sort((left, right) => right.revision - left.revision)
    } catch {
      if (isCurrent(value, route.workspaceID, route.sessionID, signal) && active?.id === note.id && valueForHistory === historyGeneration) historyError = "The note history could not be loaded. Try again."
    } finally {
      if (isCurrent(value, route.workspaceID, route.sessionID, signal) && valueForHistory === historyGeneration) historyLoading = false
    }
  }

  function selectRevision(revision: number): void {
    if (active === null) return
    const route = currentRoute
    historyDialog?.close()
    runtime.navigate(revision === active.revision ? detailPath(route.workspaceID, route.sessionID, active.id) : revisionPath(route.workspaceID, route.sessionID, active.id, revision))
  }

  function closeHistory(): void {
    historyGeneration += 1
    historyError = ""
  }

  async function logout(): Promise<void> {
    try {
      await signOut()
    } finally {
      runtime.requireLogin()
    }
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <main class="status-page" aria-busy="true" aria-live="polite"><section class="status-card"><p class="eyebrow">Gatehouse</p><div class="loading-mark" aria-hidden="true"></div><p>{auth.state.status === "checking" ? "Checking your session." : "Loading your workspaces."}</p></section></main>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <main class="status-page"><section class="status-card"><p class="eyebrow">Gatehouse</p><h1 class="title is-3">Connection unavailable</h1><p class="subtitle is-6">Gatehouse could not load your account.</p><button class="button is-primary" type="button" onclick={() => void runtime.refresh()}>Try again</button></section></main>
{:else if auth.state.status !== "authenticated"}
  <main class="status-page"><section class="status-card"><p class="eyebrow">Gatehouse</p><h1 class="title is-3">Sign in required</h1><button class="button is-primary" type="button" onclick={() => runtime.requireLogin()}>Sign in</button></section></main>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <main class="status-page"><section class="status-card"><p class="eyebrow">Gatehouse</p><h1 class="title is-3">No workspace access</h1><p class="subtitle is-6">Ask an administrator to add you to a workspace group.</p><button class="button is-primary" type="button" onclick={() => runtime.navigate("/app/no-access", true)}>Continue</button></section></main>
{:else}
  <WorkspaceFrame {mobileMenuOpen} brandTheme onMenuClose={() => mobileMenuOpen = false}>
    {#snippet sidebar()}<RouterLink class="brand" href="/app/">Gatehouse</RouterLink><div class="workspace-switcher"><label for="workspace">Workspace</label><div class="select is-fullwidth"><select id="workspace" value={workspace.id} onchange={(event) => runtime.navigate(workspacePath(event.currentTarget.value))}>{#each access.state.workspaces as candidate (candidate.id)}<option value={candidate.id}>{candidate.name ?? candidate.id}</option>{/each}</select></div></div><nav class="sidebar-nav" aria-label="Workspace navigation"><section class="sidebar-section"><ul><li><RouterLink class="active" href={sessionsPath(workspace.id)}>Chats</RouterLink></li><li><RouterLink href={`${workspacePath(workspace.id)}/prj`}>Projects</RouterLink></li><li><RouterLink href={`${workspacePath(workspace.id)}/grp`}>Groups</RouterLink></li></ul></section></nav>{#if access.state.systemAccess === "available"}<div class="sidebar-system-link"><RouterLink href="/app/system" target="_blank" rel="noopener">System</RouterLink></div>{/if}<div class="sidebar-footer"><span>{auth.state.claims?.principal.name ?? "User"}</span><button class="button is-small is-danger is-light" type="button" onclick={() => void logout()}>Log out</button></div>{/snippet}
    {#snippet header()}<button class="mobile-menu-trigger" type="button" aria-label="Open navigation menu" aria-expanded={mobileMenuOpen} onclick={() => mobileMenuOpen = true}><Menu size={20} strokeWidth={2} aria-hidden="true" /></button><h1 class="workspace-breadcrumb"><RouterLink class="workspace-breadcrumb-segment" href={workspacePath(workspace.id)}><span>{workspace.name ?? workspace.id}</span></RouterLink>{#if session !== null}{#if session.project !== undefined}<span class="workspace-breadcrumb-separator" aria-hidden="true">/</span><RouterLink class="workspace-breadcrumb-segment" href={projectPath(workspace.id, session.project.id)}><span>{session.project.name ?? "New Project"}</span></RouterLink>{/if}<span class="workspace-breadcrumb-separator" aria-hidden="true">/</span><RouterLink class="workspace-breadcrumb-segment" href={sessionPath(workspace.id, session.id)}><span>{session.name ?? "New Chat"}</span></RouterLink><span class="workspace-breadcrumb-separator" aria-hidden="true">/</span>{#if currentRoute.kind === "session-notes"}<span>Notes</span>{:else}<RouterLink href={listPath(workspace.id, session.id)}>Notes</RouterLink><span class="workspace-breadcrumb-separator" aria-hidden="true">/</span><span class="workspace-breadcrumb-segment">{currentRoute.kind === "session-note-new" ? "New Note" : selectedRevision === null ? active?.title ?? "Note" : `Revision ${selectedRevision.revision}`}</span>{/if}{/if}</h1>{#if session !== null}<SessionNavigation workspaceID={workspace.id} sessionID={session.id} active="notes" />{/if}{/snippet}
    <section class="project-note-page">
      {#if sessionStatus === "checking"}<p class="dashboard-empty" aria-busy="true" aria-live="polite">Loading chat...</p>
      {:else if sessionStatus === "unavailable"}<p class="dashboard-empty">This chat could not be loaded.</p><button class="button is-primary" type="button" onclick={() => { const signal = abortController?.signal; if (signal !== undefined) void loadRoute(currentRoute, generation, signal) }}>Try again</button>
      {:else if editing}<form class="project-note-editor" onsubmit={(event) => { event.preventDefault(); void save() }}><div class="project-note-page-heading"><div><p class="eyebrow">Session Note</p><h2>{creating ? "New Note" : "Edit Note"}</h2></div></div><div class="field"><label class="label" for="session-note-title">Title</label><div class="control"><input class="input" id="session-note-title" autocomplete="off" maxlength="256" required bind:value={title} /></div></div><div class="field"><label class="label" for="session-note-description">Description (optional)</label><div class="control"><textarea class="textarea" id="session-note-description" autocomplete="off" rows="3" maxlength="4096" bind:value={description}></textarea></div></div><div class="field"><label class="label" for="session-note-body">Content (optional)</label><div class="control"><textarea class="textarea project-note-body-input" id="session-note-body" autocomplete="off" rows="18" maxlength="1048576" bind:value={body}></textarea></div></div><div class="field"><label class="checkbox"><input type="checkbox" autocomplete="off" bind:checked={sensitive} /> Sensitive: content is marked sensitive when agents read it.</label></div>{#if error !== ""}<p class="help is-danger" aria-live="polite">{error}</p>{/if}<div class="project-note-actions"><button class="button" type="button" disabled={saving} onclick={cancelEdit}>Cancel</button><button class="button is-primary" type="submit" disabled={saving}>{saving ? "Saving..." : "Save note"}</button></div></form>
      {:else if (currentRoute.kind === "session-note" || currentRoute.kind === "session-note-edit" || currentRoute.kind === "session-note-revision") && detailStatus === "checking"}<p class="dashboard-empty">Loading note...</p>
      {:else if (currentRoute.kind === "session-note" || currentRoute.kind === "session-note-edit" || currentRoute.kind === "session-note-revision") && detailStatus === "unavailable"}<p class="dashboard-empty">Note unavailable.</p><button class="button is-primary" type="button" onclick={() => { const signal = abortController?.signal; if (signal !== undefined) void loadContent(currentRoute, generation, signal) }}>Try again</button>
      {:else if active !== null}{@const displayed = selectedRevision ?? active}<article class="project-note-view"><header class="project-note-page-heading"><div><p class="eyebrow">{selectedRevision === null ? "Session Note" : `Session Note Revision ${selectedRevision.revision}`}</p><h2><span class="project-note-title">{displayed.title}{#if displayed.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span></h2>{#if displayed.description !== ""}<p>{displayed.description}</p>{/if}<small>By {authorLabel(displayed.author)} on {dateLabel(displayed.created_at)}</small></div><div class="project-note-actions">{#if selectedRevision === null}<button class="button is-small" type="button" onclick={() => active !== null && runtime.navigate(editPath(currentRoute.workspaceID, currentRoute.sessionID, active.id))}>Edit</button>{:else}<button class="button is-small" type="button" onclick={() => active !== null && runtime.navigate(detailPath(currentRoute.workspaceID, currentRoute.sessionID, active.id))}>Current revision</button>{/if}<button class="button is-small" type="button" onclick={() => void openHistory()}>History</button>{#if selectedRevision === null}<button class="button is-small is-danger is-light" type="button" disabled={deleting} onclick={() => void remove()}>{deleting ? "Removing..." : "Remove"}</button>{/if}</div></header>{#if displayed.body !== undefined && displayed.body !== ""}<div class="markdown-content project-note-markdown">{@html renderMarkdown(displayed.body)}</div>{/if}{#if error !== ""}<p class="help is-danger" aria-live="polite">{error}</p>{/if}</article>
      {:else}<div class="collection-heading"><h2>Session Notes</h2><button class="button is-primary is-small" type="button" onclick={() => runtime.navigate(`${listPath(workspace.id, currentRoute.sessionID)}/new`)}>New note</button></div><div class="collection-list">{#if notesStatus === "checking"}<p class="dashboard-empty">Loading notes...</p>{:else if notesStatus === "unavailable"}<p class="dashboard-empty">Notes could not be loaded.</p><button class="button is-primary is-small" type="button" onclick={() => { const signal = abortController?.signal; if (signal !== undefined) void loadNotes(generation, currentRoute.workspaceID, currentRoute.sessionID, signal) }}>Try again</button>{:else}{#each notes as note (note.id)}<RouterLink class="dashboard-row project-note-row" href={detailPath(workspace.id, currentRoute.sessionID, note.id)}><span class="dashboard-row-content"><span class="project-note-title">{note.title}{#if note.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span>{#if note.description !== ""}<span class="project-note-description">{note.description}</span>{/if}<span class="dashboard-row-meta"><span>{authorLabel(note.author)}</span><time datetime={note.created_at}>{dateLabel(note.created_at)}</time></span></span></RouterLink>{:else}<p class="dashboard-empty">No notes yet.</p>{/each}{/if}</div>{/if}
    </section>
  </WorkspaceFrame>
  <ModalDialog bind:dialog={historyDialog} brandTheme labelledBy="session-note-history-heading" onClose={closeHistory}>
    {#snippet header()}<div class="note-history-heading"><h2 id="session-note-history-heading">Revision history</h2><button class="button is-ghost is-small" type="button" aria-label="Close" onclick={() => historyDialog?.close()}><X size={18} strokeWidth={2} aria-hidden="true" /></button></div>{/snippet}
    {#if historyLoading}<p class="dashboard-empty">Loading history...</p>{:else}{#if historyError !== ""}<p class="help is-danger" aria-live="polite">{historyError}</p>{/if}<div class="collection-list note-history-list">{#each history as revision (revision.revision)}<button class="dashboard-row project-note-row" class:is-selected={selectedRevision?.revision === revision.revision} type="button" onclick={() => selectRevision(revision.revision)}><span class="dashboard-row-content"><span class="project-note-title">Revision {revision.revision}: {revision.title}{#if revision.sensitive}<span class="sensitive-note-badge">Sensitive</span>{/if}</span>{#if revision.description !== ""}<span class="project-note-description">{revision.description}</span>{/if}<span class="dashboard-row-meta"><span>{authorLabel(revision.author)}</span><time datetime={revision.created_at}>{dateLabel(revision.created_at)}</time></span></span></button>{:else}<p class="dashboard-empty">No revisions found.</p>{/each}</div>{/if}
  </ModalDialog>
{/if}
