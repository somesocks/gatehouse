import { systemAdministration } from "./system"
import { fetchGatehouse } from "./api"

export type GroupScope = "system" | "workspace"
export type GroupRole = "member" | "manager"

export function groupAdministration(
  path: string,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
  scope: GroupScope = "system",
): Promise<Response> {
  if (scope === "system")
    return systemAdministration(path, method, body, signal)
  return fetchGatehouse(`/api/v1/${path}`, {
    method,
    credentials: "same-origin",
    headers:
      body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  })
}

export type SystemGroup = {
  id: string
  workspace: string
  alias?: string
  name?: string
  enabled: boolean
  revision: number
  role?: GroupRole
}

export type SystemGroupMember = {
  principal: string
  role: GroupRole
  alias?: string
  name?: string
  principal_enabled: boolean
  enabled: boolean
  revision: number
}

export type SystemGroupAgent = {
  agent: string
  alias: string
  label?: string
  agent_enabled: boolean
  enabled: boolean
  revision: number
}

export function groupAdminPath(workspace: string, group?: string, suffix = "") {
  return `workspaces/${encodeURIComponent(workspace)}/groups${group ? `/${encodeURIComponent(group)}` : ""}${suffix}`
}

export function fetchAdminGroupMemberOptions(
  workspace: string,
  group: string,
  name: string,
  cursor: string,
  signal: AbortSignal,
  scope: GroupScope = "system",
) {
  return groupAdministration(
    `${groupAdminPath(workspace, group, "/member-options")}?${new URLSearchParams({ limit: "50", name, cursor })}`,
    "GET",
    undefined,
    signal,
    scope,
  )
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
  scope: GroupScope = "system",
) {
  return groupAdministration(
    `${groupAdminPath(workspace, group, "/members")}?${new URLSearchParams({ limit: "50", name, cursor, status, active: "true" })}`,
    "GET",
    undefined,
    signal,
    scope,
  )
}

export function fetchAdminGroupAgents(
  workspace: string,
  group: string,
  name: string,
  cursor: string,
  status: string,
  signal?: AbortSignal,
  scope: GroupScope = "system",
) {
  return groupAdministration(
    `${groupAdminPath(workspace, group, "/agents")}?${new URLSearchParams({ limit: "50", name, cursor, status, active: "true" })}`,
    "GET",
    undefined,
    signal,
    scope,
  )
}
