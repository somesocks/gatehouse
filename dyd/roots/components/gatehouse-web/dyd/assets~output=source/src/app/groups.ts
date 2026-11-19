import { collectionQueryPath, type CollectionQuery } from "./collection-query"
import { fetchGatehouse as fetch } from "./api"
import type { GroupRole } from "./group-administration"

export type Group = {
  id: string
  name?: string
  alias?: string
  enabled: boolean
  role: GroupRole
}

export function fetchGroupCapabilities(
  workspaceID: string,
  signal?: AbortSignal,
) {
  return fetch(
    `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/group-capabilities`,
    { credentials: "same-origin", signal },
  )
}

export async function fetchWorkspaceGroupMemberships(
  workspaceID: string,
  signal: AbortSignal,
): Promise<Group[]> {
  const groups: Group[] = []
  const seen = new Set<string>()
  let cursor = ""
  do {
    const response = await fetchWorkspaceGroups(workspaceID, signal, { limit: 100, cursor })
    if (!response.ok) throw new Error("Groups could not be loaded.")
    const page = (await response.json()) as Group[]
    groups.push(...page.filter((group) => group.enabled))
    cursor = response.headers.get("X-Next-Cursor") ?? ""
    if (cursor && seen.has(cursor)) throw new Error("Groups could not be loaded.")
    seen.add(cursor)
  } while (cursor)
  return groups
}

export async function fetchWorkspaceGroups(
  workspaceID: string,
  signal?: AbortSignal,
  query?: CollectionQuery,
): Promise<Response> {
  return await fetch(
    collectionQueryPath(
      `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/groups`,
      query,
    ),
    { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) },
  )
}
