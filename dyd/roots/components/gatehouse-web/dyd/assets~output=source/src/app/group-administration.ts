import { systemAdministration, type WorkspaceRole } from "./system"

export type SystemGroup = {
  id: string
  workspace: string
  alias?: string
  name?: string
  enabled: boolean
  revision: number
}

export type SystemGroupMember = {
  principal: string
  alias?: string
  name?: string
  principal_enabled: boolean
  enabled: boolean
  revision: number
}

export type SystemGroupGrant = {
  id: string
  workspace: string
  group: string
  role: WorkspaceRole
  enabled: boolean
  revision: number
}

export function groupAdminPath(workspace: string, group?: string, suffix = "") {
  return `workspaces/${encodeURIComponent(workspace)}/groups${group ? `/${encodeURIComponent(group)}` : ""}${suffix}`
}

export function fetchAdminGroups(
  workspace: string,
  name: string,
  cursor: string,
  status: string,
  signal?: AbortSignal,
) {
  return systemAdministration(
    `${groupAdminPath(workspace)}?${new URLSearchParams({ limit: "50", name, cursor, status })}`,
    "GET",
    undefined,
    signal,
  )
}

export function fetchAdminGroupMembers(
  workspace: string,
  group: string,
  name: string,
  cursor: string,
  status: string,
  signal?: AbortSignal,
) {
  return systemAdministration(
    `${groupAdminPath(workspace, group, "/members")}?${new URLSearchParams({ limit: "50", name, cursor, status })}`,
    "GET",
    undefined,
    signal,
  )
}
