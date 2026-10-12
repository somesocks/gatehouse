// Shared cursor/navigation behavior. Resource controllers provide their own
// typed fetch function and response decoding; this does not define a record model.
export function createDirectoryController<T>({
  fetchPage,
  onAuthenticationLost,
  onSystemAccessChange = () => {},
}: {
  fetchPage: (
    name: string,
    cursor: string,
    status: string,
    signal: AbortSignal,
  ) => Promise<{ response: Response; items?: T[]; next?: string }>
  onAuthenticationLost: () => void
  onSystemAccessChange?: (next: "available" | "denied" | "unavailable") => void
}) {
  const state = $state({
    items: [] as T[],
    search: "",
    status: "",
    loading: true,
    error: "",
    cursor: "",
    next: "",
    history: [] as string[],
  })
  let request: AbortController | undefined
  let appliedSearch = ""
  let appliedStatus = ""
  async function page(cursor: string): Promise<boolean> {
    request?.abort()
    const current = new AbortController()
    request = current
    state.loading = true
    state.error = ""
    try {
      const result = await fetchPage(
        appliedSearch,
        cursor,
        appliedStatus,
        current.signal,
      )
      if (current.signal.aborted) return false
      if (result.response.status === 401) {
        onAuthenticationLost()
        return false
      }
      if (result.response.status === 403) {
        state.error = "You do not have access to these records."
        onSystemAccessChange("denied")
        return false
      }
      if (!result.response.ok) {
        state.error = "Records could not be loaded. Try again."
        return false
      }
      state.items = result.items ?? []
      state.next = result.next ?? ""
      state.cursor = cursor
      return true
    } catch {
      if (!current.signal.aborted)
        state.error = "Records could not be loaded. Try again."
      return false
    } finally {
      if (request === current) state.loading = false
    }
  }
  async function search() {
    appliedSearch = state.search
    appliedStatus = state.status
    state.history = []
    state.next = ""
    state.cursor = ""
    state.items = []
    return page("")
  }
  async function next() {
    if (!state.next || state.loading) return
    const previous = state.cursor
    if (await page(state.next)) state.history = [...state.history, previous]
  }
  async function previous() {
    if (!state.history.length || state.loading) return
    const target = state.history[state.history.length - 1]
    if (await page(target)) state.history = state.history.slice(0, -1)
  }
  function stop() {
    request?.abort()
  }
  return {
    state,
    search,
    next,
    previous,
    stop,
    reload: () => page(state.cursor),
  }
}
