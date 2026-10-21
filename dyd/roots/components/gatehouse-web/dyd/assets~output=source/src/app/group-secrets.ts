import { fetchGatehouse } from "./api"

export type GroupSecret = {
  id: string
  group: string
  description: string
  author: { id: string; name?: string }
  revision: number
  created_at: string
  updated_at: string
}

export type GroupSecretPage = {
  secrets: GroupSecret[]
  next_cursor?: string
  can_manage_secrets: boolean
}

function path(workspace: string, group: string, id?: string) {
  const base = `/api/v1/workspaces/${encodeURIComponent(workspace)}/groups/${encodeURIComponent(group)}/secrets`
  return id === undefined ? base : `${base}/${encodeURIComponent(id)}`
}

export function fetchGroupSecrets(workspace: string, group: string, name: string, cursor: string, signal: AbortSignal) {
  const query = new URLSearchParams({ limit: "50" })
  if (name) query.set("name", name)
  if (cursor) query.set("cursor", cursor)
  return fetchGatehouse(`${path(workspace, group)}?${query}`, { credentials: "same-origin", signal })
}

export function fetchGroupSecret(workspace: string, group: string, id: string, signal: AbortSignal) {
  return fetchGatehouse(path(workspace, group, id), { credentials: "same-origin", signal })
}

export function createGroupSecret(workspace: string, group: string, description: string, value: string, signal: AbortSignal) {
  return fetchGatehouse(path(workspace, group), {
    method: "POST", credentials: "same-origin", signal,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ description, value }),
  })
}

export function updateGroupSecret(workspace: string, group: string, secret: GroupSecret, description: string, value: string, signal: AbortSignal) {
  return fetchGatehouse(path(workspace, group, secret.id), {
    method: "PATCH", credentials: "same-origin", signal,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ description, ...(value === "" ? {} : { value }), expected_revision: secret.revision }),
  })
}

export function removeGroupSecret(workspace: string, group: string, secret: GroupSecret, signal: AbortSignal) {
  return fetchGatehouse(path(workspace, group, secret.id), {
    method: "DELETE", credentials: "same-origin", signal,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ expected_revision: secret.revision }),
  })
}
