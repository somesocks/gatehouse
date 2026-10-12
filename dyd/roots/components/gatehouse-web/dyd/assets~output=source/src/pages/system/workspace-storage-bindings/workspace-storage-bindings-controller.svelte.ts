import type { ActivityClient } from "../../../app/activity"
import {
  fetchSystemWorkspaceStorageBindings,
  systemAdministration,
  type SystemWorkspaceStorageProvider,
} from "../../../app/system"
import { createLoadState } from "../../../utils/load-state.svelte"

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
  const resource = createLoadState()
  const mutation = createLoadState()
  let appliedSearch = ""
  const state = $state({
    bindings: [] as SystemWorkspaceStorageProvider[],
    error: "",
    search: "",
    cursor: "",
    get loading() {
      return resource.loading
    },
    get reloading() {
      return resource.reloading
    },
    loadingMore: false,
    get saving() {
      return mutation.state.pending
    },
    form: { provider: "", priority: 1, enabled: true },
  })
  let unsubscribe: (() => void) | undefined

  async function load(): Promise<boolean> {
    const request = resource.begin()
    const search = state.search
    state.error = ""
    try {
      const response = await fetchSystemWorkspaceStorageBindings(
        workspaceID,
        search,
        "",
        request.signal,
      )
      if (!request.current()) return false
      if (response.status === 401) {
        onAuthenticationLost()
        return false
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return false
      }
      if (!response.ok) {
        state.error = "Workspace storage bindings could not be loaded."
        request.fail(new Error(state.error))
        return false
      }
      const result = (await response.json()) as {
        bindings: SystemWorkspaceStorageProvider[]
        next_cursor?: string
      }
      return request.commit(() => {
        state.bindings = result.bindings
        state.cursor = result.next_cursor ?? ""
        appliedSearch = search
      })
    } catch {
      if (request.current()) {
        state.error = "Workspace storage bindings could not be loaded."
        request.fail(new Error(state.error))
      }
      return false
    } finally {
      request.finish()
    }
  }

  async function loadMore(): Promise<void> {
    if (state.cursor === "" || state.loadingMore) return
    const request = resource.begin()
    state.loadingMore = true
    state.error = ""
    try {
      const response = await fetchSystemWorkspaceStorageBindings(
        workspaceID,
        appliedSearch,
        state.cursor,
        request.signal,
      )
      if (!request.current()) return
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
      if (!request.current()) return
      const seen = new Set(state.bindings.map((binding) => binding.provider))
      state.bindings = [
        ...state.bindings,
        ...result.bindings.filter((binding) => !seen.has(binding.provider)),
      ]
      state.cursor = result.next_cursor ?? ""
    } catch {
      if (request.current())
        state.error = "Workspace storage bindings could not be loaded."
    } finally {
      if (request.current()) state.loadingMore = false
      request.finish()
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
    if (state.saving) return false
    const request = mutation.begin()
    state.error = ""
    try {
      const response = await systemAdministration(
        path,
        method,
        body,
        request.signal,
      )
      if (!request.current()) return false
      if (response.status === 401) {
        onAuthenticationLost()
        return false
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return false
      }
      if (response.status === 409) {
        await load()
        if (!request.current()) return false
        state.error = "This binding changed elsewhere. Your draft is preserved."
        return false
      }
      if (!response.ok) {
        state.error = "Workspace storage binding could not be saved."
        return false
      }
      resource.stop()
      if (method === "PATCH") await load()
      return request.current()
    } catch {
      if (request.current())
        state.error = "Workspace storage binding could not be saved."
      return false
    } finally {
      request.finish()
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
      async () => {
        if (!(await load())) throw new Error("Storage binding refresh failed")
      },
    )
    return stop
  }

  function stop(): void {
    resource.stop()
    mutation.stop()
    unsubscribe?.()
    unsubscribe = undefined
  }

  return { state, load, loadMore, search, create, setEnabled, start, stop }
}
