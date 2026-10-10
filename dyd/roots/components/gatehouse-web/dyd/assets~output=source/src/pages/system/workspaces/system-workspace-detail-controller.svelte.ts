import {
  fetchSystemWorkspace,
  updateSystemWorkspace,
  type SystemWorkspace,
} from "../../../app/system"

export function createSystemWorkspaceDetailController({
  onAuthenticationLost,
  onSystemAccessChange,
}: {
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const state = $state({
    workspace: null as SystemWorkspace | null,
    form: { name: "", enabled: true },
    loading: true,
    editing: false,
    saving: false,
    error: "",
  })
  let generation = 0
  let request: AbortController | undefined

  async function loadWorkspace(
    workspaceID: string,
    currentGeneration: number,
    signal: AbortSignal,
  ): Promise<void> {
    state.loading = true
    state.error = ""
    try {
      const response = await fetchSystemWorkspace(workspaceID, signal)
      if (currentGeneration !== generation || signal.aborted) return
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return
      }
      if (response.status === 404) {
        state.workspace = null
        state.error = "Workspace was not found."
        return
      }
      if (!response.ok) {
        state.error = "Workspace could not be loaded."
        return
      }
      const workspace = (await response.json()) as SystemWorkspace
      if (currentGeneration !== generation || signal.aborted) return
      state.workspace = workspace
      state.form = {
        name: workspace.name ?? "",
        enabled: workspace.enabled,
      }
      state.editing = false
    } catch {
      if (!signal.aborted) state.error = "Workspace could not be loaded."
    } finally {
      if (currentGeneration === generation) state.loading = false
    }
  }

  function load(workspaceID: string): () => void {
    const currentGeneration = ++generation
    request?.abort()
    request = new AbortController()
    state.workspace = null
    state.form = { name: "", enabled: true }
    state.editing = false
    state.saving = false
    void loadWorkspace(workspaceID, currentGeneration, request.signal)
    return () => {
      if (currentGeneration === generation) {
        generation += 1
        request?.abort()
        state.saving = false
      }
    }
  }

  function beginEdit(): void {
    if (state.workspace === null) return
    state.form = {
      name: state.workspace.name ?? "",
      enabled: state.workspace.enabled,
    }
    state.error = ""
    state.editing = true
  }

  function cancelEdit(): void {
    state.error = ""
    state.editing = false
  }

  async function save(workspaceID: string): Promise<void> {
    if (state.saving) return
    const currentGeneration = generation
    state.saving = true
    state.error = ""
    try {
      const response = await updateSystemWorkspace(workspaceID, {
        name: state.form.name,
        enabled: state.form.enabled,
      })
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
        state.workspace = null
        state.editing = false
        state.error = "Workspace was not found."
        return
      }
      if (!response.ok) {
        state.error = "Workspace changes could not be saved."
        return
      }
      const workspace = (await response.json()) as SystemWorkspace
      if (currentGeneration !== generation) return
      state.workspace = workspace
      state.form = {
        name: state.workspace.name ?? "",
        enabled: state.workspace.enabled,
      }
      state.editing = false
    } catch {
      if (currentGeneration === generation)
        state.error = "Workspace changes could not be saved."
    } finally {
      if (currentGeneration === generation) state.saving = false
    }
  }

  return { state, load, beginEdit, cancelEdit, save }
}
