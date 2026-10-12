import {
  fetchSystemWorkspace,
  updateSystemWorkspace,
  type SystemWorkspace,
} from "../../../app/system"
import { createLoadState } from "../../../utils/load-state.svelte"

export function createSystemWorkspaceDetailController({
  onAuthenticationLost,
  onSystemAccessChange,
}: {
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const resource = createLoadState()
  const errors = $state({ value: "" })
  let owner = ""
  const state = $state({
    workspace: null as SystemWorkspace | null,
    form: { name: "", enabled: true },
    get loading() {
      return resource.loading
    },
    get reloading() {
      return resource.reloading
    },
    saving: false,
    get error() {
      return errors.value || resource.state.error
    },
    set error(value: string) {
      errors.value = value
    },
  })
  let generation = 0
  let request: AbortController | undefined

  async function loadWorkspace(
    workspaceID: string,
    currentGeneration: number,
    signal: AbortSignal,
  ): Promise<void> {
    const token = resource.begin(signal)
    signal = token.signal
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
        throw new Error("Workspace was not found.")
      }
      if (!response.ok) {
        throw new Error("Workspace could not be loaded.")
      }
      const workspace = (await response.json()) as SystemWorkspace
      if (currentGeneration !== generation || signal.aborted) return
      const first = !resource.state.hasLoaded
      token.commit(() => {
        state.workspace = workspace
        if (first)
          state.form = {
            name: workspace.name ?? "",
            enabled: workspace.enabled,
          }
      })
    } catch (error) {
      token.fail(error)
    } finally {
      token.finish()
    }
  }

  function load(workspaceID: string): () => void {
    const currentGeneration = ++generation
    request?.abort()
    request = new AbortController()
    if (owner !== workspaceID) {
      owner = workspaceID
      resource.reset()
      state.workspace = null
      state.form = { name: "", enabled: true }
    }
    state.saving = false
    void loadWorkspace(workspaceID, currentGeneration, request.signal)
    return () => {
      if (currentGeneration === generation) {
        generation += 1
        request?.abort()
        resource.stop()
        state.saving = false
      }
    }
  }

  async function save(workspaceID: string): Promise<boolean> {
    if (state.saving) return false
    const currentGeneration = generation
    state.saving = true
    state.error = ""
    try {
      const response = await updateSystemWorkspace(
        workspaceID,
        {
          name: state.form.name,
          enabled: state.form.enabled,
        },
        request?.signal,
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
        state.workspace = null
        state.error = "Workspace was not found."
        return false
      }
      if (!response.ok) {
        state.error = "Workspace changes could not be saved."
        return false
      }
      const workspace = (await response.json()) as SystemWorkspace
      if (currentGeneration !== generation) return false
      resource.stop()
      state.workspace = workspace
      state.form = { name: workspace.name ?? "", enabled: workspace.enabled }
      return true
    } catch {
      if (currentGeneration === generation)
        state.error = "Workspace changes could not be saved."
      return false
    } finally {
      if (currentGeneration === generation) state.saving = false
    }
  }

  return {
    state,
    load,
    save,
    reload: () =>
      owner && request
        ? loadWorkspace(owner, generation, request.signal)
        : Promise.resolve(),
  }
}
