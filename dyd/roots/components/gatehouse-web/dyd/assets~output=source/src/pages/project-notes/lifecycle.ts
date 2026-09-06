export type ProjectNotesListStatus = "checking" | "ready" | "unavailable"
export type ProjectNotesRouteKind = "project-notes" | "project-note-new" | "project-note"

export type ProjectNotesRouteState = {
  notesStatus: ProjectNotesListStatus
  detailStatus: ProjectNotesListStatus
  creating: boolean
  editing: boolean
}

export function activateProjectNotesRoute(kind: ProjectNotesRouteKind): ProjectNotesRouteState {
  const creating = kind === "project-note-new"
  return { notesStatus: "checking", detailStatus: kind === "project-note" ? "checking" : "ready", creating, editing: creating }
}

export function settledProjectNotesListStatus(loaded: boolean): Exclude<ProjectNotesListStatus, "checking"> {
  return loaded ? "ready" : "unavailable"
}

export function acceptsProjectNotesResult(currentGeneration: number, resultGeneration: number, currentWorkspaceID: string, resultWorkspaceID: string, currentProjectID: string, resultProjectID: string): boolean {
  return currentGeneration === resultGeneration && currentWorkspaceID === resultWorkspaceID && currentProjectID === resultProjectID
}
