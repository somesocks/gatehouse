import {
  fetchProjects,
  type ProjectSearchResponse,
  type ProjectSummary,
} from "../../app/projects"

type ProjectCollectionControllerOptions = {
  onAuthenticationLost: () => void
}

export function createProjectCollectionController({
  onAuthenticationLost,
}: ProjectCollectionControllerOptions) {
  const state = $state({
    projects: [] as ProjectSummary[],
    search: "",
    cursor: null as string | null,
    loading: false,
    error: "",
    history: [] as string[],
    page: "",
  })
  let generation = 0

  async function load(
    workspaceID: string,
    search: string,
    reset: boolean,
    signal: AbortSignal,
    currentGeneration = generation,
    target?: string,
  ): Promise<boolean> {
    if ((state.loading && !reset) || (!reset && state.cursor === null)) {
      return false
    }
    const cursor = target ?? (reset ? state.page : (state.cursor ?? ""))
    if (reset) {
      state.search = search
    }
    state.loading = true
    state.error = ""
    try {
      const response = await fetchProjects(workspaceID, search, cursor, signal)
      if (currentGeneration !== generation || signal.aborted) {
        return false
      }
      if (response.status === 401) {
        onAuthenticationLost()
        return false
      }
      if (!response.ok) {
        throw new Error("projects could not be searched")
      }
      const loaded = (await response.json()) as ProjectSearchResponse
      if (currentGeneration !== generation || signal.aborted) {
        return false
      }
      state.projects = loaded.projects
      state.cursor = loaded.next_cursor ?? null
      state.page = cursor
      return true
    } catch (error) {
      if (currentGeneration === generation && !signal.aborted)
        state.error =
          error instanceof Error
            ? error.message
            : "Projects could not be loaded."
      return false
    } finally {
      if (currentGeneration === generation) {
        state.loading = false
      }
    }
  }

  function start(
    workspaceID: string,
    search: string,
    signal: AbortSignal,
  ): () => void {
    const currentGeneration = ++generation
    state.page = ""
    state.history = []
    state.projects = []
    state.cursor = null
    void load(workspaceID, search, true, signal, currentGeneration)
    return stop
  }

  async function loadMore(
    workspaceID: string,
    search: string,
    signal: AbortSignal,
  ): Promise<void> {
    if (state.loading || state.cursor === null) return
    const previous = state.page
    if (await load(workspaceID, search, false, signal))
      state.history.push(previous)
  }

  async function previous(
    workspaceID: string,
    search: string,
    signal: AbortSignal,
  ) {
    if (state.loading || state.history.length === 0) return
    const target = state.history[state.history.length - 1]
    if (await load(workspaceID, search, true, signal, generation, target))
      state.history.pop()
  }

  function stop(): void {
    generation += 1
    state.loading = false
  }

  return { state, start, loadMore, previous, stop }
}
