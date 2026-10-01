export function capabilityFromFragment(fragment: string): string | null {
  const values = new URLSearchParams(fragment.startsWith("#") ? fragment.slice(1) : fragment)
  const credentials = values.getAll("capability")
  return values.size === 1 && credentials.length === 1 && credentials[0] !== ""
    ? credentials[0]
    : null
}

export function createInputTransport(capability: string) {
  const headers = { Authorization: `Bearer ${capability}` }
  return {
    openCustom: (path: string[]) =>
      fetch(`/api/v1/input/fields/open?${new URLSearchParams({ path: JSON.stringify(path) })}`, {
        credentials: "omit", cache: "no-store", headers,
      }),
    read: () =>
      fetch("/api/v1/input", {
        credentials: "omit",
        cache: "no-store",
        headers,
      }),
    patch: (body: string) =>
      fetch("/api/v1/input/draft", {
        method: "PATCH",
        credentials: "omit",
        cache: "no-store",
        headers: { ...headers, "Content-Type": "application/json" },
        body,
      }),
    createFile: (path: string[], file: File) =>
      fetch("/api/v1/input/files", {
        method: "POST", credentials: "omit", cache: "no-store",
        headers: { ...headers, "Content-Type": "application/json" },
        body: JSON.stringify({ path, name: file.name, ...(file.type ? { media_type: file.type } : {}) }),
      }),
    uploadFile: (url: string, file: File) =>
      fetch(url, {
        method: "PUT", credentials: "omit", cache: "no-store", body: file,
        ...(file.type ? { headers: { "Content-Type": file.type } } : {}),
      }),
    finishFile: (path: string[], id: string) =>
      fetch(`/api/v1/input/files/${encodeURIComponent(id)}/finish`, {
        method: "POST", credentials: "omit", cache: "no-store",
        headers: { ...headers, "Content-Type": "application/json" },
        body: JSON.stringify({ path }),
      }),
    terminal: (action: "submit" | "cancel") =>
      fetch(`/api/v1/input/${action}`, {
        method: "POST",
        credentials: "omit",
        cache: "no-store",
        headers,
      }),
  }
}

export type InputTransport = ReturnType<typeof createInputTransport>
