import type { ActivityClient } from "../../app/activity"
import { createSessionNote, fetchSessionNote, fetchSessionNoteRevision, fetchSessionNotes, removeSessionNote, updateSessionNote, type SessionNote } from "../../app/session-notes"
import type { Route } from "../../route"

type SessionNotesRoute = Extract<Route, { kind: "session-notes" | "session-note-new" | "session-note" | "session-note-edit" | "session-note-revision" }>
type PageStatus = "checking" | "ready" | "not-found" | "unavailable"
type Options = { activity: ActivityClient; onAuthenticationLost: () => void; onNavigate: (path: string, replace?: boolean) => void; onResetHistory: () => void }

export function createSessionNotesController({ activity, onAuthenticationLost, onNavigate, onResetHistory }: Options) {
  const state = $state({ notes: [] as SessionNote[], status: "checking" as "checking" | "ready" | "unavailable", pageStatus: "checking" as PageStatus, active: null as SessionNote | null, revision: null as SessionNote | null, creating: false, editing: false, saving: false, deleting: false, title: "", description: "", body: "", sensitive: false, error: "" })
  let context: { workspaceID: string; sessionID: string } | null = null
  let generation = 0
  let unsubscribe: (() => void) | undefined
  const listPath = (workspaceID: string, sessionID: string) => `/app/wsp/${encodeURIComponent(workspaceID)}/ses/${encodeURIComponent(sessionID)}/notes`
  const detailPath = (workspaceID: string, sessionID: string, noteID: string) => `${listPath(workspaceID, sessionID)}/${encodeURIComponent(noteID)}`
  const revisionPath = (workspaceID: string, sessionID: string, noteID: string, revision: number) => `${detailPath(workspaceID, sessionID, noteID)}/revisions/${encodeURIComponent(revision)}`
  const current = (value: number) => value === generation
  const validContext = (value: number, workspaceID: string, sessionID: string) => current(value) && context?.workspaceID === workspaceID && context?.sessionID === sessionID

  function clearSelection(): void { state.active = null; state.revision = null; state.editing = false }

  async function loadList(value: number, showLoading = false): Promise<boolean> {
    if (context === null) return false
    const { workspaceID, sessionID } = context
    if (showLoading) state.status = "checking"
    try {
      const response = await fetchSessionNotes(workspaceID, sessionID)
      if (!validContext(value, workspaceID, sessionID)) return false
      if (response.status === 401) { onAuthenticationLost(); return false }
      if (!response.ok) throw new Error("session notes could not be loaded")
      const notes = await response.json() as SessionNote[]
      if (!validContext(value, workspaceID, sessionID)) return false
      state.notes = [...notes].sort((left, right) => {
        const difference = new Date(right.created_at).getTime() - new Date(left.created_at).getTime()
        return Number.isFinite(difference) && difference !== 0 ? difference : right.id.localeCompare(left.id)
      })
      state.status = "ready"
      return true
    } catch { if (current(value)) state.status = "unavailable"; return false }
  }

  async function loadDetail(noteID: string, value: number): Promise<SessionNote | null> {
    if (context === null) return null
    const { workspaceID, sessionID } = context
    state.pageStatus = "checking"
    try {
      const response = await fetchSessionNote(workspaceID, sessionID, noteID)
      if (!validContext(value, workspaceID, sessionID)) return null
      if (response.status === 401) { onAuthenticationLost(); return null }
      if (response.status === 404) { state.pageStatus = "not-found"; return null }
      if (!response.ok) throw new Error("session note could not be loaded")
      const note = await response.json() as SessionNote
      if (!validContext(value, workspaceID, sessionID)) return null
      state.active = note
      state.pageStatus = "ready"
      return note
    } catch { if (current(value)) state.pageStatus = "unavailable"; return null }
  }

  async function loadRevision(noteID: string, revision: number, value: number): Promise<boolean> {
    if (context === null) return false
    const { workspaceID, sessionID } = context
    state.pageStatus = "checking"
    try {
      const response = await fetchSessionNoteRevision(workspaceID, sessionID, noteID, revision)
      if (!validContext(value, workspaceID, sessionID)) return false
      if (response.status === 401) { onAuthenticationLost(); return false }
      if (response.status === 404) { state.pageStatus = "not-found"; return true }
      if (!response.ok) throw new Error("session note revision could not be loaded")
      const loaded = await response.json() as SessionNote
      if (!validContext(value, workspaceID, sessionID)) return false
      state.revision = loaded
      state.pageStatus = "ready"
      return true
    } catch { if (current(value)) state.pageStatus = "unavailable"; return false }
  }

  async function refresh(route: SessionNotesRoute, value: number, showLoading = false): Promise<boolean> {
    const listLoaded = await loadList(value, showLoading)
    if (!listLoaded || !current(value)) {
      if (current(value) && route.kind !== "session-notes" && route.kind !== "session-note-new" && state.status === "unavailable") state.pageStatus = "unavailable"
      return false
    }
    if (route.kind === "session-notes" || route.kind === "session-note-new") return true
    const note = await loadDetail(route.noteID, value)
    if (note === null) return state.pageStatus !== "unavailable"
    if (route.kind === "session-note-edit" && !state.editing) { activateEdit(); return true }
    return route.kind !== "session-note-revision" || await loadRevision(route.noteID, route.revision, value)
  }

  function start(workspaceID: string, sessionID: string, route: SessionNotesRoute): () => void {
    stop(); context = { workspaceID, sessionID }; const value = ++generation
    state.notes = []; clearSelection(); state.creating = route.kind === "session-note-new"; state.editing = state.creating; state.saving = false; state.deleting = false; state.title = ""; state.description = ""; state.body = ""; state.sensitive = false; state.error = ""; state.status = "checking"; state.pageStatus = route.kind === "session-note-new" ? "ready" : "checking"
    onResetHistory()
    void refresh(route, value, true)
    unsubscribe = activity.subscribe([{ name: "session-note", topic: `${workspaceID}/${sessionID}`, events: ["session_note.*"] }], async ({ signal }) => {
      if (signal.aborted || !await refresh(route, value) || signal.aborted) throw new Error("session notes refresh failed")
    })
    return stop
  }

  function stop(): void {
    generation += 1; unsubscribe?.(); unsubscribe = undefined; context = null; onResetHistory()
    state.notes = []; state.active = null; state.revision = null; state.creating = false; state.editing = false; state.saving = false; state.deleting = false; state.title = ""; state.description = ""; state.body = ""; state.sensitive = false; state.error = ""
  }

  function activateEdit(): void {
    if (state.active === null) return
    state.title = state.active.title; state.description = state.active.description; state.body = state.active.body ?? ""; state.sensitive = state.active.sensitive; state.error = ""; state.editing = true
  }
  function startCreate(): void { if (context !== null) onNavigate(`${listPath(context.workspaceID, context.sessionID)}/new`) }
  function startEdit(): void { if (context !== null && state.active !== null) onNavigate(`${detailPath(context.workspaceID, context.sessionID, state.active.id)}/edit`) }
  function cancelEdit(): void {
    if (state.saving || context === null) return
    state.error = ""
    onNavigate(state.creating ? listPath(context.workspaceID, context.sessionID) : state.active === null ? listPath(context.workspaceID, context.sessionID) : detailPath(context.workspaceID, context.sessionID, state.active.id))
  }
  function showCurrentRevision(): void { if (context !== null && state.active !== null) onNavigate(detailPath(context.workspaceID, context.sessionID, state.active.id)) }
  function openRevision(revision: number): void { if (context !== null && state.active !== null) onNavigate(revisionPath(context.workspaceID, context.sessionID, state.active.id, revision)) }

  async function save(): Promise<void> {
    if (context === null || state.title.trim() === "") { state.error = "Title is required."; return }
    const { workspaceID, sessionID } = context; const value = generation; const active = state.active; const input = { title: state.title, description: state.description, body: state.body, sensitive: state.sensitive }
    state.error = ""; state.saving = true
    try {
      const response = state.creating ? await createSessionNote(workspaceID, sessionID, input) : active === null ? undefined : await updateSessionNote(workspaceID, sessionID, active.id, input)
      if (response === undefined || !validContext(value, workspaceID, sessionID)) return
      if (response.status === 401) { onAuthenticationLost(); return }
      if (!response.ok) throw new Error("session note could not be saved")
      const saved = await response.json() as SessionNote
      if (!validContext(value, workspaceID, sessionID)) return
      state.active = saved; state.revision = null; state.creating = false; state.editing = false; state.notes = [saved, ...state.notes.filter((note) => note.id !== saved.id)]; onResetHistory(); onNavigate(detailPath(workspaceID, sessionID, saved.id))
    } catch { if (current(value)) state.error = "The note could not be saved. Try again." } finally { if (current(value)) state.saving = false }
  }

  async function remove(): Promise<void> {
    if (context === null || state.active === null || state.deleting || !window.confirm(`Remove ${state.active.title}?`)) return
    const { workspaceID, sessionID } = context; const value = generation; const note = state.active
    state.deleting = true; state.error = ""
    try {
      const response = await removeSessionNote(workspaceID, sessionID, note.id)
      if (!validContext(value, workspaceID, sessionID) || state.active?.id !== note.id) return
      if (response.status === 401) { onAuthenticationLost(); return }
      if (!response.ok) throw new Error("session note could not be removed")
      clearSelection(); state.notes = state.notes.filter((candidate) => candidate.id !== note.id); onResetHistory(); onNavigate(listPath(workspaceID, sessionID))
    } catch { if (current(value)) state.error = "The note could not be removed. Try again." } finally { if (current(value)) state.deleting = false }
  }

  return { state, start, stop, startCreate, startEdit, cancelEdit, showCurrentRevision, openRevision, save, remove }
}
