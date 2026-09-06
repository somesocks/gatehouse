export type Chat = {
  id: string
  created_at: string
  name?: string
  project?: { id: string; name?: string }
}

export type ChatSearchResponse = {
  sessions: Chat[]
  next_cursor?: string
}

export async function fetchChats(workspaceID: string, name: string, cursor: string): Promise<Response> {
  const parameters = new URLSearchParams({ limit: "50" })
  if (name.trim() !== "") {
    parameters.set("name", name)
  }
  if (cursor !== "") {
    parameters.set("cursor", cursor)
  }
  return await fetch(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/sessions?${parameters}`, { credentials: "same-origin" })
}

export async function fetchProjectChats(workspaceID: string, projectID: string, signal?: AbortSignal): Promise<Response> {
  const parameters = new URLSearchParams({ limit: "5", project: projectID })
  return await fetch(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/sessions?${parameters}`, { credentials: "same-origin", signal })
}

export async function createProjectChat(workspaceID: string, projectID: string, signal?: AbortSignal): Promise<Response> {
  return await fetch(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/sessions`, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ project: projectID }), signal })
}
