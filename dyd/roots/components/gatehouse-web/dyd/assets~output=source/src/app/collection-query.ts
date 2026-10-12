export type CollectionQuery = { name?: string; cursor?: string; limit: number }

export function collectionQueryPath(
  path: string,
  query?: CollectionQuery,
): string {
  if (!query) return path
  const params = new URLSearchParams({ limit: String(query.limit) })
  if (query.name) params.set("name", query.name)
  if (query.cursor) params.set("cursor", query.cursor)
  return `${path}?${params}`
}
