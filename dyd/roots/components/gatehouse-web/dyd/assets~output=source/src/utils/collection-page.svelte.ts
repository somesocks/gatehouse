import type { CollectionQuery } from "../app/collection-query"

// Pagination state only: resource pages own their transport and data models.
export function createCollectionPage() {
  let pending: { query: string; cursor: string; next: string } | undefined
  const continuation = $state({ next: "" })
  const state = $state({
    search: "",
    query: "",
    cursor: "",
    get next() {
      return continuation.next
    },
    set next(value: string) {
      if (pending) pending.next = value
      else continuation.next = value
    },
    history: [] as string[],
    busy: false,
  })
  let generation = 0
  const key = () =>
    `${pending?.query ?? state.query}\n${pending?.cursor ?? state.cursor}`
  const params = (limit = 50): CollectionQuery => ({
    name: pending?.query ?? state.query,
    cursor: pending?.cursor ?? state.cursor,
    limit,
  })
  const accept = (response: Response) => {
    const next = response.headers.get("X-Next-Cursor") ?? ""
    if (pending) pending.next = next
    else state.next = next
  }
  function reset() {
    generation++
    pending = undefined
    Object.assign(state, {
      search: "",
      query: "",
      cursor: "",
      next: "",
      history: [],
      busy: false,
    })
  }
  async function go(
    cursor: string,
    reload: () => Promise<boolean>,
    query = state.query,
    history = state.history,
  ) {
    if (state.busy) return false
    const value = generation
    pending = { cursor, query, next: "" }
    state.busy = true
    try {
      const loaded = await reload()
      if (!loaded || value !== generation || !pending) return false
      const committed = pending
      pending = undefined
      Object.assign(state, { ...committed, history })
      return true
    } finally {
      if (value === generation) {
        pending = undefined
        state.busy = false
      }
    }
  }
  async function search(reload: () => Promise<boolean>) {
    if (state.busy) return
    return await go("", reload, state.search.trim(), [])
  }
  async function next(reload: () => Promise<boolean>) {
    if (state.busy || !state.next) return
    const previous = state.cursor
    return await go(state.next, reload, state.query, [
      ...state.history,
      previous,
    ])
  }
  async function previous(reload: () => Promise<boolean>) {
    if (state.busy || !state.history.length) return
    return await go(
      state.history[state.history.length - 1],
      reload,
      state.query,
      state.history.slice(0, -1),
    )
  }
  return { state, key, params, accept, reset, search, next, previous }
}
