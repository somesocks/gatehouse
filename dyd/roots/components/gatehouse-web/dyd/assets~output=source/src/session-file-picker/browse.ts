export type FileBrowse = {
  name: string
  mediaType: string
  limit: number
  cursor: string
  direction: "next" | "previous"
}

export function fileBrowseFromSearch(search: string): FileBrowse {
  const query = new URLSearchParams(search)
  const limit = Number(query.get("limit") ?? "25")
  const cursor = query.get("cursor") ?? ""
  return {
    name: query.get("name") ?? "", mediaType: query.get("media_type") ?? "",
    limit: Number.isInteger(limit) && limit >= 1 && limit <= 100 ? limit : 25,
    cursor, direction: cursor !== "" && query.get("direction") === "previous" ? "previous" : "next",
  }
}

export function fileBrowseParameters(browse: FileBrowse): URLSearchParams {
  const query = new URLSearchParams()
  if (browse.name.trim()) query.set("name", browse.name.trim())
  if (browse.mediaType.trim()) query.set("media_type", browse.mediaType.trim())
  if (browse.limit !== 25) query.set("limit", String(browse.limit))
  if (browse.cursor) {
    query.set("cursor", browse.cursor)
    if (browse.direction === "previous") query.set("direction", browse.direction)
  }
  return query
}

// The credential remains in the fragment; only browsing state enters the query.
export function fileBrowseURL(current: string, browse: FileBrowse): string {
  const url = new URL(current)
  url.search = fileBrowseParameters(browse).toString()
  return url.href
}
