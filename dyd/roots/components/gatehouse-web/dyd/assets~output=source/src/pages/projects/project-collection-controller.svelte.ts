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
  })
  let generation = 0

  async function load(
    workspaceID: string,
    search: string,
    reset: boolean,
    signal: AbortSignal,
    currentGeneration = generation,
  ): Promise<void> {
    if ((state.loading && !reset) || (!reset && state.cursor === null)) {
      return
    }
    const cursor = reset ? "" : (state.cursor ?? "")
    if (reset) {
      state.search = search
      state.projects = []
      state.cursor = null
    }
    state.loading = true
    try {
      const response = await fetchProjects(workspaceID, search, cursor, signal)
      if (currentGeneration !== generation || signal.aborted) {
        return
      }
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (!response.ok) {
        throw new Error("projects could not be searched")
      }
      const loaded = (await response.json()) as ProjectSearchResponse
      if (currentGeneration !== generation || signal.aborted) {
        return
      }
      state.projects = reset
        ? loaded.projects
        : [...state.projects, ...loaded.projects]
      state.cursor = loaded.next_cursor ?? null
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
    void load(workspaceID, search, true, signal, currentGeneration)
    return stop
  }

  function loadMore(
    workspaceID: string,
    search: string,
    signal: AbortSignal,
  ): void {
    void load(workspaceID, search, false, signal)
  }

  function stop(): void {
    generation += 1
    state.loading = false
  }

  return { state, start, loadMore, stop }
}
