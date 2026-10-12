import type { CollectionQuery } from "../app/collection-query"

// Pagination state only: resource pages own their transport and data models.
export function createCollectionPage() {
  const state = $state({
    search: "",
    query: "",
    cursor: "",
    next: "",
    history: [] as string[],
    busy: false,
  })
  let generation = 0
  const key = () => `${state.query}\n${state.cursor}`
  const params = (limit = 50): CollectionQuery => ({
    name: state.query,
    cursor: state.cursor,
    limit,
  })
  const accept = (response: Response) => {
    state.next = response.headers.get("X-Next-Cursor") ?? ""
  }
  function reset() {
    generation++
    Object.assign(state, {
      search: "",
      query: "",
      cursor: "",
      next: "",
      history: [],
      busy: false,
    })
  }
  async function go(cursor: string, reload: () => Promise<boolean>) {
    if (state.busy) return false
    const value = generation
    const previous = { cursor: state.cursor, next: state.next }
    state.busy = true
    state.cursor = cursor
    try {
      if (await reload()) return value === generation
      if (value === generation) Object.assign(state, previous)
      return false
    } finally {
      if (value === generation) state.busy = false
    }
  }
  async function search(reload: () => Promise<boolean>) {
    if (state.busy) return
    state.query = state.search.trim()
    state.history = []
    state.cursor = ""
    state.next = ""
    await go("", reload)
  }
  async function next(reload: () => Promise<boolean>) {
    if (state.busy || !state.next) return
    const previous = state.cursor
    if (await go(state.next, reload)) state.history.push(previous)
  }
  async function previous(reload: () => Promise<boolean>) {
    if (state.busy || !state.history.length) return
    if (await go(state.history[state.history.length - 1], reload))
      state.history.pop()
  }
  return { state, key, params, accept, reset, search, next, previous }
}
