export type ProjectSecret = {
  id: string
  description: string
  author: { id: string; name?: string }
  created_at: string
  updated_at: string
}

function path(workspaceID: string, projectID: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/projects/${encodeURIComponent(projectID)}/secrets`
}

export async function fetchProjectSecrets(workspaceID: string, projectID: string, signal?: AbortSignal): Promise<Response> { return await fetch(path(workspaceID, projectID), { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) }) }
export async function fetchProjectSecret(workspaceID: string, projectID: string, secretID: string, signal?: AbortSignal): Promise<Response> { return await fetch(`${path(workspaceID, projectID)}/${encodeURIComponent(secretID)}`, { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) }) }
export async function createProjectSecret(workspaceID: string, projectID: string, input: { description: string; value: string }, signal?: AbortSignal): Promise<Response> { return await fetch(path(workspaceID, projectID), { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input), ...(signal === undefined ? {} : { signal }) }) }
export async function updateProjectSecret(workspaceID: string, projectID: string, secretID: string, input: { description: string; value?: string }, signal?: AbortSignal): Promise<Response> { return await fetch(`${path(workspaceID, projectID)}/${encodeURIComponent(secretID)}`, { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input), ...(signal === undefined ? {} : { signal }) }) }
export async function removeProjectSecret(workspaceID: string, projectID: string, secretID: string, signal?: AbortSignal): Promise<Response> { return await fetch(`${path(workspaceID, projectID)}/${encodeURIComponent(secretID)}`, { method: "DELETE", credentials: "same-origin", ...(signal === undefined ? {} : { signal }) }) }
