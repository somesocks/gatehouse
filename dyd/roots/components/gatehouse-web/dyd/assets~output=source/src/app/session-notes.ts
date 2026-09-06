import type { NoteAuthor } from "./project-notes"

export type SessionNote = {
  id: string
  title: string
  description: string
  body?: string
  sensitive: boolean
  author: NoteAuthor
  created_at: string
  revision: number
}

export type SessionNoteInput = {
  title: string
  description: string
  body: string
  sensitive: boolean
}

export function sessionNotesAPIPath(workspaceID: string, sessionID: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/sessions/${encodeURIComponent(sessionID)}/notes`
}

export async function fetchSessionNotes(workspaceID: string, sessionID: string, signal?: AbortSignal): Promise<Response> {
  return await fetch(sessionNotesAPIPath(workspaceID, sessionID), { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) })
}

export async function fetchSessionNote(workspaceID: string, sessionID: string, noteID: string, signal?: AbortSignal): Promise<Response> {
  return await fetch(`${sessionNotesAPIPath(workspaceID, sessionID)}/${encodeURIComponent(noteID)}`, { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) })
}

export async function fetchSessionNoteRevisions(workspaceID: string, sessionID: string, noteID: string, signal?: AbortSignal): Promise<Response> {
  return await fetch(`${sessionNotesAPIPath(workspaceID, sessionID)}/${encodeURIComponent(noteID)}/revisions`, { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) })
}

export async function fetchSessionNoteRevision(workspaceID: string, sessionID: string, noteID: string, revision: number, signal?: AbortSignal): Promise<Response> {
  return await fetch(`${sessionNotesAPIPath(workspaceID, sessionID)}/${encodeURIComponent(noteID)}/revisions/${encodeURIComponent(revision)}`, { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) })
}

export async function createSessionNote(workspaceID: string, sessionID: string, input: SessionNoteInput, signal?: AbortSignal): Promise<Response> {
  return await fetch(sessionNotesAPIPath(workspaceID, sessionID), { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input), ...(signal === undefined ? {} : { signal }) })
}

export async function updateSessionNote(workspaceID: string, sessionID: string, noteID: string, input: SessionNoteInput, signal?: AbortSignal): Promise<Response> {
  return await fetch(`${sessionNotesAPIPath(workspaceID, sessionID)}/${encodeURIComponent(noteID)}`, { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input), ...(signal === undefined ? {} : { signal }) })
}

export async function removeSessionNote(workspaceID: string, sessionID: string, noteID: string, signal?: AbortSignal): Promise<Response> {
  return await fetch(`${sessionNotesAPIPath(workspaceID, sessionID)}/${encodeURIComponent(noteID)}`, { method: "DELETE", credentials: "same-origin", ...(signal === undefined ? {} : { signal }) })
}
