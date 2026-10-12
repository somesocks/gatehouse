import { createLoadState } from "./load-state.svelte"

// Shared request/cursor mechanics; each caller owns its typed native transport.
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
  const lifecycle = createLoadState()
  const state = $state({
    items: [] as T[],
    search: "",
    status: "",
    cursor: "",
    next: "",
    history: [] as string[],
    get loading() {
      return lifecycle.loading
    },
    get reloading() {
      return lifecycle.reloading
    },
    get busy() {
      return lifecycle.state.pending
    },
    get hasLoaded() {
      return lifecycle.state.hasLoaded
    },
    get error() {
      return lifecycle.state.error
    },
    set error(value: string) {
      lifecycle.state.error = value
    },
  })
  let appliedSearch = ""
  let appliedStatus = ""
  let attempted:
    | { cursor: string; name: string; status: string; history: string[] }
    | undefined
  async function page(
    cursor: string,
    name = appliedSearch,
    status = appliedStatus,
    history = state.history,
  ): Promise<boolean> {
    attempted = { cursor, name, status, history: [...history] }
    const request = lifecycle.begin()
    try {
      const result = await fetchPage(name, cursor, status, request.signal)
      if (!request.current()) return false
      if (result.response.status === 401) {
        onAuthenticationLost()
        throw new Error("Sign in is required.")
      }
      if (result.response.status === 403) {
        onSystemAccessChange("denied")
        throw new Error("You do not have access to these records.")
      }
      if (!result.response.ok)
        throw new Error("Records could not be loaded. Try again.")
      return request.commit(() => {
        state.items = result.items ?? []
        state.next = result.next ?? ""
        state.cursor = cursor
        appliedSearch = name
        appliedStatus = status
        state.history = history
      })
    } catch (error) {
      request.fail(error)
      return false
    } finally {
      request.finish()
    }
  }
  const search = () => page("", state.search.trim(), state.status, [])
  async function next() {
    if (!state.next || state.busy) return
    const previous = state.cursor
    await page(state.next, appliedSearch, appliedStatus, [
      ...state.history,
      previous,
    ])
  }
  async function previous() {
    if (!state.history.length || state.busy) return
    const target = state.history[state.history.length - 1]
    await page(target, appliedSearch, appliedStatus, state.history.slice(0, -1))
  }
  function reset() {
    attempted = undefined
    lifecycle.reset()
    appliedSearch = ""
    appliedStatus = ""
    Object.assign(state, {
      items: [],
      search: "",
      status: "",
      cursor: "",
      next: "",
      history: [],
    })
  }
  return {
    state,
    search,
    next,
    previous,
    reset,
    stop: lifecycle.stop,
    reload: () => page(state.cursor),
    retry: () =>
      attempted
        ? page(
            attempted.cursor,
            attempted.name,
            attempted.status,
            attempted.history,
          )
        : search(),
    lifecycle,
  }
}
