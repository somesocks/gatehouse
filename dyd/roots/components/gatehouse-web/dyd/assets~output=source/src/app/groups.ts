export type Group = {
  id: string
  name?: string
}

export async function fetchWorkspaceGroups(workspaceID: string): Promise<Response> {
  return await fetch(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/groups`, { credentials: "same-origin" })
}
