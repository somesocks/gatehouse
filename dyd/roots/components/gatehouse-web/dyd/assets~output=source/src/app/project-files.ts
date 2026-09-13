export type ProjectFile = {
  id: string
  name: string
  media_type?: string
  size: number
  fingerprint: string
  created_at: string
}

function path(workspaceID: string, projectID: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/projects/${encodeURIComponent(projectID)}/files`
}

export async function fetchProjectFiles(
  workspaceID: string,
  projectID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(path(workspaceID, projectID), {
    credentials: "same-origin",
    signal,
  })
}

export async function startProjectFileUpload(
  workspaceID: string,
  projectID: string,
  file: File,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(`${path(workspaceID, projectID)}/start`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      name: file.name,
      ...(file.type === "" ? {} : { media_type: file.type }),
    }),
    signal,
  })
}

export async function finishProjectFileUpload(
  workspaceID: string,
  projectID: string,
  fileID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${path(workspaceID, projectID)}/${encodeURIComponent(fileID)}/finish`,
    { method: "POST", credentials: "same-origin", signal },
  )
}

export async function removeProjectFile(
  workspaceID: string,
  projectID: string,
  fileID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${path(workspaceID, projectID)}/${encodeURIComponent(fileID)}`,
    { method: "DELETE", credentials: "same-origin", signal },
  )
}

export function projectFileDownloadPath(
  workspaceID: string,
  projectID: string,
  fileID: string,
): string {
  return `${path(workspaceID, projectID)}/${encodeURIComponent(fileID)}/download`
}
