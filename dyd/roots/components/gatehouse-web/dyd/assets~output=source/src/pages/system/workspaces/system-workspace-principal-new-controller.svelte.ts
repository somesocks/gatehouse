import {
  createSystemWorkspacePrincipalGrant,
  fetchSystemWorkspacePrincipalOptions,
  type SystemWorkspacePrincipalOption,
  type WorkspaceRole,
} from "../../../app/system"

export function createSystemWorkspacePrincipalNewController({
  onAuthenticationLost,
  onSystemAccessChange,
}: {
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const state = $state({
    principals: [] as SystemWorkspacePrincipalOption[],
    cursor: "",
    search: "",
    appliedSearch: "",
    selectedPrincipalID: "",
    role: "member" as WorkspaceRole,
    loading: false,
    loadingMore: false,
    adding: false,
    error: "",
  })
  let generation = 0
  let request: AbortController | undefined
  let lifetime = new AbortController()

  function load(): () => void {
    lifetime.abort()
    lifetime = new AbortController()
    const currentGeneration = ++generation
    request?.abort()
    state.principals = []
    state.cursor = ""
    state.search = ""
    state.appliedSearch = ""
    state.selectedPrincipalID = ""
    state.role = "member"
    state.error = ""
    return () => {
      if (currentGeneration === generation) {
        generation += 1
        request?.abort()
        lifetime.abort()
      }
    }
  }

  async function search(workspaceID: string, append = false): Promise<void> {
    if (append && state.loadingMore) return
    const cursor = append ? state.cursor : ""
    const name = append ? state.appliedSearch : state.search
    const currentGeneration = generation
    request?.abort()
    const currentRequest = new AbortController()
    request = currentRequest
    if (append) state.loadingMore = true
    else {
      state.loading = true
    }
    state.error = ""
    try {
      const response = await fetchSystemWorkspacePrincipalOptions(
        workspaceID,
        name,
        cursor,
        currentRequest.signal,
      )
      if (currentGeneration !== generation || currentRequest.signal.aborted)
        return
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        state.principals = []
        return
      }
      if (response.status === 404) {
        state.error = "Workspace was not found."
        return
      }
      if (!response.ok) {
        state.error = "Principal search could not be loaded."
        return
      }
      const result = (await response.json()) as {
        principals: SystemWorkspacePrincipalOption[]
        next_cursor?: string
      }
      if (currentGeneration !== generation || currentRequest.signal.aborted)
        return
      if (append) {
        const seen = new Set(state.principals.map((principal) => principal.id))
        state.principals = [
          ...state.principals,
          ...result.principals.filter((principal) => !seen.has(principal.id)),
        ]
      } else state.principals = result.principals
      state.cursor = result.next_cursor ?? ""
      state.appliedSearch = name
    } catch {
      if (!currentRequest.signal.aborted)
        state.error = "Principal search could not be loaded."
    } finally {
      if (currentGeneration === generation && currentRequest === request) {
        if (append) state.loadingMore = false
        else state.loading = false
      }
    }
  }

  async function add(workspaceID: string): Promise<boolean> {
    if (state.adding || state.selectedPrincipalID === "") return false
    const currentGeneration = generation
    state.adding = true
    state.error = ""
    try {
      const response = await createSystemWorkspacePrincipalGrant(
        workspaceID,
        state.selectedPrincipalID,
        state.role,
        lifetime.signal,
      )
      if (currentGeneration !== generation) return false
      if (response.status === 401) {
        onAuthenticationLost()
        return false
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return false
      }
      if (response.status === 404) {
        state.error = "Workspace or principal was not found."
        return false
      }
      if (response.status === 409) {
        state.error = "That principal already has this direct role."
        return false
      }
      if (!response.ok) {
        state.error = "Principal could not be added to the workspace."
        return false
      }
      return true
    } catch {
      if (currentGeneration === generation)
        state.error = "Principal could not be added to the workspace."
      return false
    } finally {
      if (currentGeneration === generation) state.adding = false
    }
  }

  return { state, load, search, add }
}
