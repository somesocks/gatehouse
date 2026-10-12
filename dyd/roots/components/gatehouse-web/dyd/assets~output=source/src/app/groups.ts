import { collectionQueryPath, type CollectionQuery } from "./collection-query"

export type Group = {
  id: string
  name?: string
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
