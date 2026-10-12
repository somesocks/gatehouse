import { fetchGatehouse as fetch } from "./api"

export type Workspace = {
  id: string
  name?: string
  description?: string
}

export async function fetchWorkspaces(signal?: AbortSignal): Promise<Response> {
  return await fetch("/api/v1/workspaces", {
    credentials: "same-origin",
    signal,
  })
}
