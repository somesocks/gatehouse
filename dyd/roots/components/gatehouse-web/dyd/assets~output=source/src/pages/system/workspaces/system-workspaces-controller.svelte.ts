import {
  createSystemWorkspace,
  fetchSystemWorkspaces,
  type SystemWorkspaceSearchResponse,
  type SystemWorkspace,
} from "../../../app/system"

export function createSystemWorkspacesController({
  onAuthenticationLost,
  onSystemAccessChange,
}: {
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const state = $state({
    workspaces: [] as SystemWorkspace[],
    search: "",
    cursor: null as string | null,
    loading: true,
    error: "",
  })

  let generation = 0
  let request: AbortController | undefined

  async function load(
    reset: boolean,
    currentGeneration: number,
    signal: AbortSignal,
  ): Promise<void> {
    if ((state.loading && !reset) || (!reset && state.cursor === null)) return
    const cursor = reset ? "" : (state.cursor ?? "")
    if (reset) {
      state.workspaces = []
      state.cursor = null
    }
    state.loading = true
    state.error = ""
    try {
      const response = await fetchSystemWorkspaces(state.search, cursor, signal)
      if (currentGeneration !== generation || signal.aborted) return
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return
      }
      if (!response.ok) {
        state.error = "Workspaces could not be loaded."
        return
      }
      const result = (await response.json()) as SystemWorkspaceSearchResponse
      if (currentGeneration !== generation || signal.aborted) return
      state.workspaces = reset
        ? result.workspaces
        : [...state.workspaces, ...result.workspaces]
      state.cursor = result.next_cursor ?? null
    } catch {
      if (currentGeneration === generation && !signal.aborted)
        state.error = "Workspaces could not be loaded."
    } finally {
      if (currentGeneration === generation) state.loading = false
    }
  }

  function restart(): void {
    const currentGeneration = ++generation
    request?.abort()
    request = new AbortController()
    void load(true, currentGeneration, request.signal)
  }

  function start(): () => void {
    restart()
    return stop
  }

  function search(): void {
    restart()
  }

  function loadMore(): void {
    if (request !== undefined) void load(false, generation, request.signal)
  }

  function stop(): void {
    generation += 1
    request?.abort()
    request = undefined
    state.loading = false
  }

  return { state, start, search, loadMore, stop }
}

export function createSystemWorkspaceFormController({
  onAuthenticationLost,
  onSystemAccessChange,
}: {
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const state = $state({
    alias: "",
    name: "",
    enabled: true,
    saving: false,
    error: "",
  })

  async function create(): Promise<boolean> {
    if (state.saving) return false
    state.error = ""
    state.saving = true
    try {
      const response = await createSystemWorkspace({
        alias: state.alias.trim(),
        ...(state.name.trim() === "" ? {} : { name: state.name.trim() }),
        enabled: state.enabled,
      })
      if (response.status === 401) {
        onAuthenticationLost()
        return false
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return false
      }
      if (response.status === 409) {
        state.error = "A workspace with this alias already exists."
        return false
      }
      if (response.status === 400) {
        state.error =
          (await response.text()) || "The workspace details are invalid."
        return false
      }
      if (!response.ok) {
        state.error = "The workspace could not be created."
        return false
      }
      return true
    } catch {
      state.error = "The workspace could not be created."
      return false
    } finally {
      state.saving = false
    }
  }

  return { state, create }
}
