import { fetchGatehouse } from "./api"
import { resourceRoles, type ResourceRole } from "./roles"

export type ProjectMemberKind = "principal" | "group"
export type ProjectRole = ResourceRole
export type ProjectGrant = {
  id: string
  principal?: string
  group?: string
  role: ProjectRole
  enabled: boolean
  revision: number
}
export type ProjectMember = {
  id: string
  alias?: string
  name?: string
  enabled: boolean
  grants: ProjectGrant[]
}
export type ProjectMemberPage = {
  members: ProjectMember[]
  next_cursor?: string
  can_manage_access: boolean
}
export const projectRoles = resourceRoles
export function projectMemberName(item: ProjectMember) {
  return item.name ?? item.alias ?? item.id
}
function path(workspaceID: string, projectID: string) {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/projects/${encodeURIComponent(projectID)}`
}
export function fetchProjectMembers(
  workspaceID: string,
  projectID: string,
  kind: ProjectMemberKind,
  name: string,
  cursor: string,
  signal: AbortSignal,
  options = false,
) {
  const query = new URLSearchParams({ kind, limit: "50" })
  if (name) query.set("name", name)
  if (cursor) query.set("cursor", cursor)
  return fetchGatehouse(
    `${path(workspaceID, projectID)}/${options ? `${kind}-options` : "members"}?${query}`,
    { credentials: "same-origin", signal },
  )
}
export function addProjectMember(
  workspaceID: string,
  projectID: string,
  kind: ProjectMemberKind,
  subject: string,
  role: ProjectRole,
  signal: AbortSignal,
) {
  return fetchGatehouse(`${path(workspaceID, projectID)}/grants`, {
    method: "POST",
    credentials: "same-origin",
    signal,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ [kind]: subject, role }),
  })
}
export function updateProjectGrant(
  workspaceID: string,
  projectID: string,
  grant: ProjectGrant,
  update: { role?: ProjectRole; enabled?: boolean },
  signal: AbortSignal,
) {
  return fetchGatehouse(`${path(workspaceID, projectID)}/grants/${encodeURIComponent(grant.id)}`, {
    method: "PATCH",
    credentials: "same-origin",
    signal,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ...update, expected_revision: grant.revision }),
  })
}
