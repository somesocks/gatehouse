export type Workspace = {
  id: string
  name?: string
}

export async function fetchWorkspaces(): Promise<Response> {
  return await fetch("/api/v1/workspaces", { credentials: "same-origin" })
}

export async function fetchSystemGrants(): Promise<Response> {
  return await fetch("/api/v1/system/grants", { credentials: "same-origin" })
}
