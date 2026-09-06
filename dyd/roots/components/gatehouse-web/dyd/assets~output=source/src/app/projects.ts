export type ProjectSummary = {
  id: string
  created_at: string
  name?: string
}

export type Project = ProjectSummary & {
  description?: string
}

export async function fetchProject(workspaceID: string, projectID: string, signal?: AbortSignal): Promise<Response> {
  return await fetch(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/projects/${encodeURIComponent(projectID)}`, { credentials: "same-origin", signal })
}

export async function updateProject(workspaceID: string, projectID: string, input: { name: string; description: string }, signal?: AbortSignal): Promise<Response> {
  return await fetch(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/projects/${encodeURIComponent(projectID)}`, { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input), signal })
}

export type ProjectSearchResponse = {
  projects: ProjectSummary[]
  next_cursor?: string
}

export async function fetchProjects(workspaceID: string, name: string, cursor: string, signal?: AbortSignal): Promise<Response> {
  const parameters = new URLSearchParams({ limit: "50" })
  if (name.trim() !== "") {
    parameters.set("name", name)
  }
  if (cursor !== "") {
    parameters.set("cursor", cursor)
  }
  return await fetch(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/projects?${parameters}`, { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) })
}

export async function createProject(workspaceID: string, signal?: AbortSignal): Promise<Response> {
  return await fetch(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/projects`, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({}), ...(signal === undefined ? {} : { signal }) })
}

export async function fetchDashboardProjects(workspaceID: string, signal?: AbortSignal): Promise<Response> {
  return await fetch(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/projects?limit=5`, { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) })
}
