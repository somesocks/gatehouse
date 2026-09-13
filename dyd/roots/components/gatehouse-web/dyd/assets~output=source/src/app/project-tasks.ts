export type TaskAuthor = {
  principal?: { id: string; name?: string }
  agent?: { id: string; label?: string }
  gateway?: string
}

export type ProjectTask = {
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

export type TaskStatus =
  | "draft"
  | "ready"
  | "in_progress"
  | "done"
  | "cancelled"

export type ProjectTaskInput = {
  title: string
  description: string
  sensitive: boolean
  status: TaskStatus
}

export function projectTasksAPIPath(
  workspaceID: string,
  projectID: string,
): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/projects/${encodeURIComponent(projectID)}/tasks`
}

export async function fetchProjectTasks(
  workspaceID: string,
  projectID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(projectTasksAPIPath(workspaceID, projectID), {
    credentials: "same-origin",
    ...(signal === undefined ? {} : { signal }),
  })
}

export async function fetchProjectTask(
  workspaceID: string,
  projectID: string,
  taskID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${projectTasksAPIPath(workspaceID, projectID)}/${encodeURIComponent(taskID)}`,
    { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) },
  )
}

export async function createProjectTask(
  workspaceID: string,
  projectID: string,
  input: ProjectTaskInput,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(projectTasksAPIPath(workspaceID, projectID), {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    ...(signal === undefined ? {} : { signal }),
  })
}

export async function updateProjectTask(
  workspaceID: string,
  projectID: string,
  taskID: string,
  input: ProjectTaskInput,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${projectTasksAPIPath(workspaceID, projectID)}/${encodeURIComponent(taskID)}`,
    {
      method: "PATCH",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
      ...(signal === undefined ? {} : { signal }),
    },
  )
}

export async function removeProjectTask(
  workspaceID: string,
  projectID: string,
  taskID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${projectTasksAPIPath(workspaceID, projectID)}/${encodeURIComponent(taskID)}`,
    {
      method: "DELETE",
      credentials: "same-origin",
      ...(signal === undefined ? {} : { signal }),
    },
  )
}
