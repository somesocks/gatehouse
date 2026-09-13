export type NoteAuthor = {
  principal?: { id: string; name?: string }
  agent?: { id: string; label?: string }
  gateway?: string
}

export type ProjectNote = {
  id: string
  title: string
  description: string
  body?: string
  sensitive: boolean
  author: NoteAuthor
  created_at: string
  revision: number
}

export type ProjectNoteInput = {
  title: string
  description: string
  body: string
  sensitive: boolean
}

export function projectNotesAPIPath(
  workspaceID: string,
  projectID: string,
): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/projects/${encodeURIComponent(projectID)}/notes`
}

export async function fetchProjectNotes(
  workspaceID: string,
  projectID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(projectNotesAPIPath(workspaceID, projectID), {
    credentials: "same-origin",
    ...(signal === undefined ? {} : { signal }),
  })
}

export async function fetchProjectNote(
  workspaceID: string,
  projectID: string,
  noteID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${projectNotesAPIPath(workspaceID, projectID)}/${encodeURIComponent(noteID)}`,
    { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) },
  )
}

export async function fetchProjectNoteRevisions(
  workspaceID: string,
  projectID: string,
  noteID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${projectNotesAPIPath(workspaceID, projectID)}/${encodeURIComponent(noteID)}/revisions`,
    { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) },
  )
}

export async function fetchProjectNoteRevision(
  workspaceID: string,
  projectID: string,
  noteID: string,
  revision: number,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${projectNotesAPIPath(workspaceID, projectID)}/${encodeURIComponent(noteID)}/revisions/${encodeURIComponent(String(revision))}`,
    { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) },
  )
}

export async function createProjectNote(
  workspaceID: string,
  projectID: string,
  input: ProjectNoteInput,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(projectNotesAPIPath(workspaceID, projectID), {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    ...(signal === undefined ? {} : { signal }),
  })
}

export async function updateProjectNote(
  workspaceID: string,
  projectID: string,
  noteID: string,
  input: ProjectNoteInput,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${projectNotesAPIPath(workspaceID, projectID)}/${encodeURIComponent(noteID)}`,
    {
      method: "PATCH",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
      ...(signal === undefined ? {} : { signal }),
    },
  )
}

export async function removeProjectNote(
  workspaceID: string,
  projectID: string,
  noteID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${projectNotesAPIPath(workspaceID, projectID)}/${encodeURIComponent(noteID)}`,
    {
      method: "DELETE",
      credentials: "same-origin",
      ...(signal === undefined ? {} : { signal }),
    },
  )
}
