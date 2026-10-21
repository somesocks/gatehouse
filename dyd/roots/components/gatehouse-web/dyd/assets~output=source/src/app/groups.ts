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
