export function createInputTransport(capability: string) {
  const headers = { Authorization: `Bearer ${capability}` }
  return {
    openCustom: (path: string[], signal?: AbortSignal) =>
      fetch(
        `/api/v1/input/fields/open?${new URLSearchParams({ path: JSON.stringify(path) })}`,
        {
          credentials: "omit",
          cache: "no-store",
          headers,
          ...(signal ? { signal } : {}),
        },
      ),
    read: (signal?: AbortSignal) =>
      fetch("/api/v1/input", {
        credentials: "omit",
        cache: "no-store",
        headers,
        ...(signal ? { signal } : {}),
      }),
    patch: (body: string, signal?: AbortSignal) =>
      fetch("/api/v1/input/draft", {
        method: "PATCH",
        credentials: "omit",
        cache: "no-store",
        headers: { ...headers, "Content-Type": "application/json" },
        body,
        ...(signal ? { signal } : {}),
      }),
    createFile: (path: string[], file: File, signal?: AbortSignal) =>
      fetch("/api/v1/input/files", {
        method: "POST",
        credentials: "omit",
        cache: "no-store",
        headers: { ...headers, "Content-Type": "application/json" },
        body: JSON.stringify({
          path,
          name: file.name,
          ...(file.type ? { media_type: file.type } : {}),
        }),
        ...(signal ? { signal } : {}),
      }),
    uploadFile: (url: string, file: File, signal?: AbortSignal) =>
      fetch(url, {
        method: "PUT",
        credentials: "omit",
        cache: "no-store",
        body: file,
        ...(file.type ? { headers: { "Content-Type": file.type } } : {}),
        ...(signal ? { signal } : {}),
      }),
    finishFile: (path: string[], id: string, signal?: AbortSignal) =>
      fetch(`/api/v1/input/files/${encodeURIComponent(id)}/finish`, {
        method: "POST",
        credentials: "omit",
        cache: "no-store",
        headers: { ...headers, "Content-Type": "application/json" },
        body: JSON.stringify({ path }),
        ...(signal ? { signal } : {}),
      }),
    terminal: (action: "submit" | "cancel", signal?: AbortSignal) =>
      fetch(`/api/v1/input/${action}`, {
        method: "POST",
        credentials: "omit",
        cache: "no-store",
        headers,
        ...(signal ? { signal } : {}),
      }),
  }
}

export type InputTransport = ReturnType<typeof createInputTransport>
