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

export type ProjectSearchResponse = {
  projects: ProjectSummary[]
  next_cursor?: string
}

export async function fetchProjects(workspaceID: string, name: string, cursor: string): Promise<Response> {
  const parameters = new URLSearchParams({ limit: "50" })
  if (name.trim() !== "") {
    parameters.set("name", name)
  }
  if (cursor !== "") {
    parameters.set("cursor", cursor)
  }
  return await fetch(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/projects?${parameters}`, { credentials: "same-origin" })
}
