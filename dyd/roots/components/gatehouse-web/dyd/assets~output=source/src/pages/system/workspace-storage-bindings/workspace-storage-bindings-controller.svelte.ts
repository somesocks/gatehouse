import type { ActivityClient } from "../../../app/activity"
import { systemAdministration, type SystemWorkspaceStorageProvider } from "../../../app/system"

type WorkspaceStorageBindingsControllerOptions = {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}

export function createWorkspaceStorageBindingsController({ activity, onAuthenticationLost, onSystemAccessChange }: WorkspaceStorageBindingsControllerOptions) {
  const state = $state({ bindings: [] as SystemWorkspaceStorageProvider[], error: "", saving: false, form: { workspace: "", provider: "", priority: 0 } })
  let unsubscribe: (() => void) | undefined

  async function load(): Promise<void> {
    state.error = ""
    try {
      const response = await systemAdministration("workspace-storage-providers")
      if (response.status === 401) { onAuthenticationLost(); return }
      if (response.status === 403) { onSystemAccessChange("denied"); return }
      if (!response.ok) { state.error = "Workspace storage bindings could not be loaded."; return }
      state.bindings = await response.json() as SystemWorkspaceStorageProvider[]
    } catch { state.error = "Workspace storage bindings could not be loaded." }
  }

  async function save(path: string, method: "POST" | "PATCH", body: Record<string, unknown>): Promise<boolean> {
    state.error = ""
    state.saving = true
    try {
      const response = await systemAdministration(path, method, body)
      if (response.status === 401) { onAuthenticationLost(); return false }
      if (response.status === 403) { onSystemAccessChange("denied"); return false }
      if (response.status === 409) { state.error = "This binding changed elsewhere. The latest settings have been reloaded."; await load(); return false }
      if (!response.ok) { state.error = "Workspace storage binding could not be saved."; return false }
      await load()
      return true
    } catch { state.error = "Workspace storage binding could not be saved."; return false } finally { state.saving = false }
  }

  async function create(): Promise<void> {
    const { form } = state
    if (!await save(`workspace-storage-providers/${encodeURIComponent(form.workspace)}/${encodeURIComponent(form.provider)}`, "POST", { priority: form.priority, enabled: true })) return
    state.form = { workspace: "", provider: "", priority: 0 }
  }

  async function setEnabled(binding: SystemWorkspaceStorageProvider, enabled: boolean): Promise<void> {
    await save(`workspace-storage-providers/${encodeURIComponent(binding.workspace)}/${encodeURIComponent(binding.provider)}`, "PATCH", { priority: binding.priority, enabled, expected_revision: binding.revision })
  }

  function start(): () => void {
    stop()
    unsubscribe = activity.subscribe([{ name: "workspace-storage-bindings", topic: "sys", events: ["workspace_storage_provider.*"] }], async () => await load())
    return stop
  }

  function stop(): void { unsubscribe?.(); unsubscribe = undefined }

  return { state, load, create, setEnabled, start, stop }
}
