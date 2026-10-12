import { fileBrowseParameters, type FileBrowse } from "./browse"

export type FieldLaunch = { api: string; capability: string }

export function fieldLaunchFromFragment(
  fragment: string,
  origin: string,
): FieldLaunch | null {
  const values = new URLSearchParams(fragment.replace(/^#/, ""))
  if (
    values.size !== 3 ||
    values.get("version") !== "1" ||
    !values.get("capability")
  )
    return null
  try {
    const api = new URL(values.get("api") ?? "")
    if (
      api.origin !== origin ||
      api.pathname !== "/api/v1/input/field" ||
      api.search ||
      api.hash ||
      api.username ||
      api.password
    )
      return null
    return { api: api.href, capability: values.get("capability")! }
  } catch {
    return null
  }
}

export function createFieldTransport(launch: FieldLaunch) {
  const headers = { Authorization: `Bearer ${launch.capability}` }
  const request = (suffix = "", options: RequestInit = {}) =>
    fetch(launch.api + suffix, {
      credentials: "omit",
      cache: "no-store",
      ...options,
      headers: { ...headers, ...options.headers },
    })
  return {
    read: (signal?: AbortSignal) => request("", { signal }),
    files: (browse: FileBrowse, accept: string[], signal?: AbortSignal) => {
      const query = fileBrowseParameters(browse)
      query.set("limit", String(browse.limit))
      for (const type of accept) query.append("accept", type)
      return request(`/files?${query}`, { signal })
    },
    references: (ids: string[], signal?: AbortSignal) => {
      const query = new URLSearchParams()
      for (const id of ids) query.append("id", id)
      return request(`/files?${query}`, { signal })
    },
    save: (ids: string[], signal?: AbortSignal) =>
      request("", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ op: "set", value: { file_ids: ids } }),
        signal,
      }),
    createFile: (file: File, signal?: AbortSignal) =>
      request("/files", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: file.name,
          ...(file.type ? { media_type: file.type } : {}),
        }),
        signal,
      }),
    upload: (url: string, file: File, signal?: AbortSignal) =>
      fetch(url, {
        method: "PUT",
        credentials: "omit",
        cache: "no-store",
        body: file,
        ...(file.type ? { headers: { "Content-Type": file.type } } : {}),
        signal,
      }),
    finishFile: (id: string, signal?: AbortSignal) =>
      request(`/files/${encodeURIComponent(id)}/finish`, {
        method: "POST",
        signal,
      }),
    download: (id: string, signal?: AbortSignal) =>
      request(`/files/${encodeURIComponent(id)}/download`, { signal }),
  }
}

export type FieldTransport = ReturnType<typeof createFieldTransport>
