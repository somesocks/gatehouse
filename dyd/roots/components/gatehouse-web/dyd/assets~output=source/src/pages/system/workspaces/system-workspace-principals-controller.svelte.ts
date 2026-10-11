import {
  fetchSystemWorkspacePrincipals,
  removeSystemWorkspacePrincipalGrants,
  type SystemWorkspacePrincipal,
} from "../../../app/system"

export function createSystemWorkspacePrincipalsController({
  onAuthenticationLost,
  onSystemAccessChange,
}: {
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const state = $state({
    principals: [] as SystemWorkspacePrincipal[],
    cursor: "",
    loading: true,
    loadingMore: false,
    search: "",
    appliedSearch: "",
    error: "",
    notice: "",
    removingIDs: new Set<string>(),
  })
  let generation = 0
  let request: AbortController | undefined

  async function fetchPage(
    workspaceID: string,
    currentGeneration: number,
    cursor: string,
    append: boolean,
    search: string,
  ): Promise<void> {
    if (append && state.loadingMore) return
    request?.abort()
    const currentRequest = new AbortController()
    request = currentRequest
    if (append) state.loadingMore = true
    else state.loading = true
    state.error = ""
    try {
      const response = await fetchSystemWorkspacePrincipals(
        workspaceID,
        search,
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
        state.principals = []
        state.error = "Workspace was not found."
        return
      }
      if (!response.ok) {
        state.error = "Workspace principals could not be loaded."
        return
      }
      const result = (await response.json()) as {
        principals: SystemWorkspacePrincipal[]
        next_cursor?: string
      }
      if (currentGeneration !== generation || currentRequest.signal.aborted)
        return
      state.principals = append
        ? mergePages(state.principals, result.principals)
        : result.principals
      state.cursor = result.next_cursor ?? ""
      state.appliedSearch = search
    } catch {
      if (!currentRequest.signal.aborted)
        state.error = "Workspace principals could not be loaded."
    } finally {
      if (currentGeneration === generation) {
        if (append) state.loadingMore = false
        else state.loading = false
      }
    }
  }

  function load(workspaceID: string): () => void {
    const currentGeneration = ++generation
    request?.abort()
    state.principals = []
    state.cursor = ""
    state.search = ""
    state.appliedSearch = ""
    state.loading = true
    state.loadingMore = false
    state.error = ""
    state.notice = ""
    state.removingIDs = new Set()
    void fetchPage(workspaceID, currentGeneration, "", false, "")
    return () => {
      if (currentGeneration === generation) {
        generation += 1
        request?.abort()
      }
    }
  }

  function search(workspaceID: string): void {
    state.notice = ""
    void fetchPage(workspaceID, generation, "", false, state.search)
  }

  function loadMore(workspaceID: string): void {
    if (state.cursor === "") return
    void fetchPage(
      workspaceID,
      generation,
      state.cursor,
      true,
      state.appliedSearch,
    )
  }

  async function remove(
    workspaceID: string,
    principal: SystemWorkspacePrincipal,
  ): Promise<void> {
    if (state.removingIDs.has(principal.id)) return
    if (
      !window.confirm(
        `Remove direct workspace access for ${principal.name ?? principal.id}? Access through groups may remain.`,
      )
    )
      return
    const currentGeneration = generation
    state.error = ""
    state.notice = ""
    state.removingIDs = new Set(state.removingIDs).add(principal.id)
    try {
      const response = await removeSystemWorkspacePrincipalGrants(
        workspaceID,
        principal.id,
      )
      if (currentGeneration !== generation) return
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return
      }
      if (response.status === 404) {
        state.error = "Workspace or principal was not found."
        return
      }
      if (!response.ok) {
        state.error = "Direct workspace access could not be removed."
        return
      }
      state.principals = state.principals.filter(
        (entry) => entry.id !== principal.id,
      )
      state.notice = "Direct workspace access was removed."
    } catch {
      if (currentGeneration === generation)
        state.error = "Direct workspace access could not be removed."
    } finally {
      if (currentGeneration === generation) {
        const next = new Set(state.removingIDs)
        next.delete(principal.id)
        state.removingIDs = next
      }
    }
  }

  return { state, load, search, loadMore, remove }
}

function mergePages(
  current: SystemWorkspacePrincipal[],
  incoming: SystemWorkspacePrincipal[],
): SystemWorkspacePrincipal[] {
  const merged = new Map(current.map((principal) => [principal.id, principal]))
  for (const principal of incoming) {
    const previous = merged.get(principal.id)
    if (previous === undefined) merged.set(principal.id, principal)
    else
      previous.roles = [
        ...new Set([...previous.roles, ...principal.roles]),
      ].sort()
  }
  return [...merged.values()]
}
