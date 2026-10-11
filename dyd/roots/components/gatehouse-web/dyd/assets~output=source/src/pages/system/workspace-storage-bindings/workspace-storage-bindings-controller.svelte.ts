import type { ActivityClient } from "../../../app/activity"
import {
  fetchSystemWorkspaceStorageBindings,
  systemAdministration,
  type SystemWorkspaceStorageProvider,
} from "../../../app/system"

type WorkspaceStorageBindingsControllerOptions = {
  activity: ActivityClient
  workspaceID: string
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}

export function createWorkspaceStorageBindingsController({
  activity,
  workspaceID,
  onAuthenticationLost,
  onSystemAccessChange,
}: WorkspaceStorageBindingsControllerOptions) {
  const state = $state({
    bindings: [] as SystemWorkspaceStorageProvider[],
    error: "",
    search: "",
    cursor: "",
    loading: true,
    loadingMore: false,
    saving: false,
    form: { provider: "", priority: 1, enabled: true },
  })
  let unsubscribe: (() => void) | undefined

  async function load(): Promise<void> {
    state.error = ""
    state.loading = true
    try {
      const response = await fetchSystemWorkspaceStorageBindings(
        workspaceID,
        state.search,
        "",
      )
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return
      }
      if (!response.ok) {
        state.error = "Workspace storage bindings could not be loaded."
        return
      }
      const result = (await response.json()) as {
        bindings: SystemWorkspaceStorageProvider[]
        next_cursor?: string
      }
      state.bindings = result.bindings
      state.cursor = result.next_cursor ?? ""
    } catch {
      state.error = "Workspace storage bindings could not be loaded."
    } finally {
      state.loading = false
    }
  }

  async function loadMore(): Promise<void> {
    if (state.cursor === "" || state.loadingMore) return
    state.loadingMore = true
    state.error = ""
    try {
      const response = await fetchSystemWorkspaceStorageBindings(
        workspaceID,
        state.search,
        state.cursor,
      )
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return
      }
      if (!response.ok) {
        state.error = "Workspace storage bindings could not be loaded."
        return
      }
      const result = (await response.json()) as {
        bindings: SystemWorkspaceStorageProvider[]
        next_cursor?: string
      }
      const seen = new Set(state.bindings.map((binding) => binding.provider))
      state.bindings = [
        ...state.bindings,
        ...result.bindings.filter((binding) => !seen.has(binding.provider)),
      ]
      state.cursor = result.next_cursor ?? ""
    } catch {
      state.error = "Workspace storage bindings could not be loaded."
    } finally {
      state.loadingMore = false
    }
  }

  function search(): void {
    void load()
  }

  async function save(
    path: string,
    method: "POST" | "PATCH",
    body: Record<string, unknown>,
  ): Promise<boolean> {
    state.error = ""
    state.saving = true
    try {
      const response = await systemAdministration(path, method, body)
      if (response.status === 401) {
        onAuthenticationLost()
        return false
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return false
      }
      if (response.status === 409) {
        state.error =
          "This binding changed elsewhere. The latest settings have been reloaded."
        await load()
        return false
      }
      if (!response.ok) {
        state.error = "Workspace storage binding could not be saved."
        return false
      }
      await load()
      return true
    } catch {
      state.error = "Workspace storage binding could not be saved."
      return false
    } finally {
      state.saving = false
    }
  }

  async function create(): Promise<void> {
    const { form } = state
    if (
      !(await save(
        `workspace-storage-providers/${encodeURIComponent(workspaceID)}/${encodeURIComponent(form.provider)}`,
        "POST",
        { priority: form.priority, enabled: form.enabled },
      ))
    )
      return
    state.form = { provider: "", priority: 1, enabled: true }
  }

  async function setEnabled(
    binding: SystemWorkspaceStorageProvider,
    enabled: boolean,
  ): Promise<void> {
    await save(
      `workspace-storage-providers/${encodeURIComponent(binding.workspace)}/${encodeURIComponent(binding.provider)}`,
      "PATCH",
      {
        priority: binding.priority,
        enabled,
        expected_revision: binding.revision,
      },
    )
  }

  function start(): () => void {
    stop()
    unsubscribe = activity.subscribe(
      [
        {
          name: "workspace-storage-bindings",
          topic: "sys",
          events: ["workspace_storage_provider.*"],
        },
      ],
      async () => await load(),
    )
    return stop
  }

  function stop(): void {
    unsubscribe?.()
    unsubscribe = undefined
  }

  return { state, load, loadMore, search, create, setEnabled, start, stop }
}
