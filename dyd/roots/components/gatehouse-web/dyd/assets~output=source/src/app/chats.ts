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
