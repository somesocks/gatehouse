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

export async function fetchSessionNotes(workspaceID: string, sessionID: string): Promise<Response> {
  return await fetch(sessionNotesAPIPath(workspaceID, sessionID), { credentials: "same-origin" })
}

export async function fetchSessionNote(workspaceID: string, sessionID: string, noteID: string): Promise<Response> {
  return await fetch(`${sessionNotesAPIPath(workspaceID, sessionID)}/${encodeURIComponent(noteID)}`, { credentials: "same-origin" })
}

export async function fetchSessionNoteRevision(workspaceID: string, sessionID: string, noteID: string, revision: number): Promise<Response> {
  return await fetch(`${sessionNotesAPIPath(workspaceID, sessionID)}/${encodeURIComponent(noteID)}/revisions/${encodeURIComponent(revision)}`, { credentials: "same-origin" })
}

export async function createSessionNote(workspaceID: string, sessionID: string, input: SessionNoteInput): Promise<Response> {
  return await fetch(sessionNotesAPIPath(workspaceID, sessionID), { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) })
}

export async function updateSessionNote(workspaceID: string, sessionID: string, noteID: string, input: SessionNoteInput): Promise<Response> {
  return await fetch(`${sessionNotesAPIPath(workspaceID, sessionID)}/${encodeURIComponent(noteID)}`, { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) })
}

export async function removeSessionNote(workspaceID: string, sessionID: string, noteID: string): Promise<Response> {
  return await fetch(`${sessionNotesAPIPath(workspaceID, sessionID)}/${encodeURIComponent(noteID)}`, { method: "DELETE", credentials: "same-origin" })
}
