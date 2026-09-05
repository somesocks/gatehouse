import type { ActivityClient } from "../../app/activity"
import { createProjectNote, fetchProjectNote, removeProjectNote, updateProjectNote, type ProjectNote } from "../../app/project-notes"
import type { Route } from "../../route"

type ProjectNotesRoute = Extract<Route, { kind: "project-notes" | "project-note-new" | "project-note" }>
type Options = {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onNavigate: (path: string, replace?: boolean) => void
  onPreviewChanged: () => Promise<boolean> | boolean
  onResetHistory: () => void
}

export function createProjectNotesController({ activity, onAuthenticationLost, onNavigate, onPreviewChanged, onResetHistory }: Options) {
  const state = $state({ active: null as ProjectNote | null, creating: false, editing: false, loading: false, saving: false, deleting: false, title: "", description: "", body: "", sensitive: false, error: "" })
  let context: { workspaceID: string; projectID: string } | null = null
  let generation = 0
  let unsubscribe: (() => void) | undefined

  const listPath = (workspaceID: string, projectID: string) => `/app/wsp/${encodeURIComponent(workspaceID)}/prj/${encodeURIComponent(projectID)}/pnt`
  const dashboardPath = (workspaceID: string, projectID: string) => `/app/wsp/${encodeURIComponent(workspaceID)}/prj/${encodeURIComponent(projectID)}`
  const detailPath = (workspaceID: string, projectID: string, noteID: string) => `${listPath(workspaceID, projectID)}/${encodeURIComponent(noteID)}`
  const current = (value: number) => value === generation

  function clearSelection(): void {
    state.active = null
    state.editing = false
  }

  async function loadDetail(noteID: string, value: number): Promise<boolean> {
    if (context === null) return false
    const { workspaceID, projectID } = context
    state.loading = true
    try {
      const response = await fetchProjectNote(workspaceID, projectID, noteID)
      if (!current(value) || context?.workspaceID !== workspaceID || context?.projectID !== projectID) return false
      if (response.status === 401) { onAuthenticationLost(); return false }
      if (response.status === 404) { clearSelection(); onNavigate(dashboardPath(workspaceID, projectID), true); return true }
      if (!response.ok) throw new Error("project note could not be loaded")
      const note = await response.json() as ProjectNote
      if (!current(value) || context?.workspaceID !== workspaceID || context?.projectID !== projectID) return false
      if (state.active?.id === note.id && state.active.revision !== note.revision) onResetHistory()
      state.active = note
      return true
    } catch {
      if (current(value)) state.error = "The note could not be loaded. Try again."
      return false
    } finally {
      if (current(value)) state.loading = false
    }
  }

  async function refresh(route: ProjectNotesRoute, value: number): Promise<boolean> {
    const previewChanged = await onPreviewChanged()
    if (!previewChanged || !current(value) || context === null) return false
    return route.kind !== "project-note" || await loadDetail(route.noteID, value)
  }

  function start(workspaceID: string, projectID: string, route: ProjectNotesRoute): () => void {
    stop()
    context = { workspaceID, projectID }
    const value = ++generation
    state.active = null
    state.creating = route.kind === "project-note-new"
    state.editing = state.creating
    state.loading = route.kind === "project-note"
    state.saving = false
    state.deleting = false
    state.title = ""
    state.description = ""
    state.body = ""
    state.sensitive = false
    state.error = ""
    if (route.kind === "project-note") void loadDetail(route.noteID, value)
    unsubscribe = activity.subscribe([{ name: "project-note", topic: `${workspaceID}/${projectID}`, events: ["project_note.*"] }], async ({ signal }) => {
      if (signal.aborted || !await refresh(route, value) || signal.aborted) throw new Error("project notes refresh failed")
    })
    return stop
  }

  function stop(): void {
    generation += 1
    unsubscribe?.()
    unsubscribe = undefined
    context = null
    state.loading = false
    state.saving = false
    state.deleting = false
  }

  function startCreate(): void {
    if (context !== null) onNavigate(`${listPath(context.workspaceID, context.projectID)}/new`)
  }

  function startEdit(): void {
    if (state.active === null) return
    state.title = state.active.title
    state.description = state.active.description
    state.body = state.active.body ?? ""
    state.sensitive = state.active.sensitive
    state.error = ""
    state.editing = true
  }

  function cancelEdit(): void {
    if (state.saving || context === null) return
    state.error = ""
    if (state.creating) {
      state.creating = false
      state.editing = false
      onNavigate(listPath(context.workspaceID, context.projectID))
      return
    }
    state.editing = false
  }

  async function save(): Promise<void> {
    if (context === null || state.title.trim() === "") { state.error = "Title is required."; return }
    const { workspaceID, projectID } = context
    const value = generation
    const active = state.active
    const input = { title: state.title, description: state.description, body: state.body, sensitive: state.sensitive }
    state.error = ""
    state.saving = true
    try {
      const response = state.creating ? await createProjectNote(workspaceID, projectID, input) : active === null ? undefined : await updateProjectNote(workspaceID, projectID, active.id, input)
      if (response === undefined || !current(value) || context?.workspaceID !== workspaceID || context?.projectID !== projectID) return
      if (response.status === 401) { onAuthenticationLost(); return }
      if (!response.ok) throw new Error("project note could not be saved")
      const saved = await response.json() as ProjectNote
      if (!current(value)) return
      if (active?.revision !== saved.revision) onResetHistory()
      state.active = saved
      state.creating = false
      state.editing = false
      await onPreviewChanged()
      if (current(value)) onNavigate(detailPath(workspaceID, projectID, saved.id))
    } catch {
      if (current(value)) state.error = "The note could not be saved. Try again."
    } finally {
      if (current(value)) state.saving = false
    }
  }

  async function remove(): Promise<void> {
    if (context === null || state.active === null || state.deleting || !window.confirm(`Remove ${state.active.title}?`)) return
    const { workspaceID, projectID } = context
    const value = generation
    const note = state.active
    state.deleting = true
    state.error = ""
    try {
      const response = await removeProjectNote(workspaceID, projectID, note.id)
      if (!current(value) || context?.workspaceID !== workspaceID || context?.projectID !== projectID || state.active?.id !== note.id) return
      if (response.status === 401) { onAuthenticationLost(); return }
      if (!response.ok) throw new Error("project note could not be removed")
      onResetHistory()
      clearSelection()
      await onPreviewChanged()
      if (current(value)) onNavigate(dashboardPath(workspaceID, projectID))
    } catch {
      if (current(value)) state.error = "The note could not be removed. Try again."
    } finally {
      if (current(value)) state.deleting = false
    }
  }

  return { state, start, stop, startCreate, startEdit, cancelEdit, save, remove }
}
