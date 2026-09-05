import { fetchChats, type Chat, type ChatSearchResponse } from "../../app/chats"

type ChatCollectionControllerOptions = {
  onAuthenticationLost: () => void
}

export function createChatCollectionController({ onAuthenticationLost }: ChatCollectionControllerOptions) {
  const state = $state({ chats: [] as Chat[], search: "", cursor: null as string | null, loading: false })
  let generation = 0

  async function load(workspaceID: string, search: string, reset: boolean, currentGeneration = generation): Promise<void> {
    if (state.loading && !reset || !reset && state.cursor === null) {
      return
    }
    const cursor = reset ? "" : state.cursor ?? ""
    if (reset) {
      state.search = search
      state.chats = []
      state.cursor = null
    }
    state.loading = true
    try {
      const response = await fetchChats(workspaceID, search, cursor)
      if (currentGeneration !== generation) {
        return
      }
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (!response.ok) {
        throw new Error("sessions could not be searched")
      }
      const loaded = await response.json() as ChatSearchResponse
      if (currentGeneration !== generation) {
        return
      }
      state.chats = reset ? loaded.sessions : [...state.chats, ...loaded.sessions]
      state.cursor = loaded.next_cursor ?? null
    } finally {
      if (currentGeneration === generation) {
        state.loading = false
      }
    }
  }

  function start(workspaceID: string, search: string): () => void {
    const currentGeneration = ++generation
    void load(workspaceID, search, true, currentGeneration)
    return stop
  }

  function loadMore(workspaceID: string, search: string): void {
    void load(workspaceID, search, false)
  }

  function stop(): void {
    generation += 1
    state.loading = false
  }

  return { state, start, loadMore, stop }
}
