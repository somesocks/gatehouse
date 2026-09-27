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
