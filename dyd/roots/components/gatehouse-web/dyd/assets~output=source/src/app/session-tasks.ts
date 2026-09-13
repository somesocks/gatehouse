import type { TaskAuthor, TaskStatus } from "./project-tasks"

export type SessionTask = {
  id: string
  title: string
  description?: string
  sensitive: boolean
  status: TaskStatus
  creator: TaskAuthor
  updater: TaskAuthor
  created_at: string
  updated_at: string
}

export type SessionTaskInput = {
  title: string
  description: string
  sensitive: boolean
  status: TaskStatus
}

export function sessionTasksAPIPath(
  workspaceID: string,
  sessionID: string,
): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/sessions/${encodeURIComponent(sessionID)}/tasks`
}

export async function fetchSessionTasks(
  workspaceID: string,
  sessionID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(sessionTasksAPIPath(workspaceID, sessionID), {
    credentials: "same-origin",
    ...(signal === undefined ? {} : { signal }),
  })
}

export async function fetchSessionTask(
  workspaceID: string,
  sessionID: string,
  taskID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${sessionTasksAPIPath(workspaceID, sessionID)}/${encodeURIComponent(taskID)}`,
    { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) },
  )
}

export async function createSessionTask(
  workspaceID: string,
  sessionID: string,
  input: SessionTaskInput,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(sessionTasksAPIPath(workspaceID, sessionID), {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    ...(signal === undefined ? {} : { signal }),
  })
}

export async function updateSessionTask(
  workspaceID: string,
  sessionID: string,
  taskID: string,
  input: SessionTaskInput,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${sessionTasksAPIPath(workspaceID, sessionID)}/${encodeURIComponent(taskID)}`,
    {
      method: "PATCH",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
      ...(signal === undefined ? {} : { signal }),
    },
  )
}

export async function removeSessionTask(
  workspaceID: string,
  sessionID: string,
  taskID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${sessionTasksAPIPath(workspaceID, sessionID)}/${encodeURIComponent(taskID)}`,
    {
      method: "DELETE",
      credentials: "same-origin",
      ...(signal === undefined ? {} : { signal }),
    },
  )
}
