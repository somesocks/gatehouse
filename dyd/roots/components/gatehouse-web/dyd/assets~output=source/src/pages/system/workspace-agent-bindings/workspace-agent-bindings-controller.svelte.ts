import type { ActivityClient } from "../../../app/activity"
import {
  fetchSystemWorkspaceAgents,
  systemAdministration,
  type SystemWorkspaceAgent,
} from "../../../app/system"

type WorkspaceAgentBindingsControllerOptions = {
  activity: ActivityClient
  workspaceID: string
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}

export function createWorkspaceAgentBindingsController({
  activity,
  workspaceID,
  onAuthenticationLost,
  onSystemAccessChange,
}: WorkspaceAgentBindingsControllerOptions) {
  const state = $state({
    bindings: [] as SystemWorkspaceAgent[],
    error: "",
    search: "",
    cursor: "",
    loading: true,
    loadingMore: false,
    saving: false,
    form: {
      alias: "",
      model: "",
      label: "",
      systemPrompt: "",
      prelude: "",
      rateLimits: "",
      default: false,
      enabled: true,
    },
  })
  let unsubscribe: (() => void) | undefined

  const optional = (value: string): string | undefined =>
    value.trim() === "" ? undefined : value.trim()

  const rateLimits = (value: string): unknown =>
    value.trim() === "" ? undefined : JSON.parse(value)

  async function load(): Promise<void> {
    state.error = ""
    state.loading = true
    try {
      const response = await fetchSystemWorkspaceAgents(
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
        state.error = "Workspace agent bindings could not be loaded."
        return
      }
      const result = (await response.json()) as {
        agents: SystemWorkspaceAgent[]
        next_cursor?: string
      }
      state.bindings = result.agents
      state.cursor = result.next_cursor ?? ""
    } catch {
      state.error = "Workspace agent bindings could not be loaded."
    } finally {
      state.loading = false
    }
  }

  async function loadMore(): Promise<void> {
    if (state.cursor === "" || state.loadingMore) return
    state.loadingMore = true
    state.error = ""
    try {
      const response = await fetchSystemWorkspaceAgents(
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
        state.error = "Workspace agent bindings could not be loaded."
        return
      }
      const result = (await response.json()) as {
        agents: SystemWorkspaceAgent[]
        next_cursor?: string
      }
      const seen = new Set(state.bindings.map((binding) => binding.id))
      state.bindings = [
        ...state.bindings,
        ...result.agents.filter((binding) => !seen.has(binding.id)),
      ]
      state.cursor = result.next_cursor ?? ""
    } catch {
      state.error = "Workspace agent bindings could not be loaded."
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
        state.error = "Workspace agent binding could not be saved."
        return false
      }
      await load()
      return true
    } catch {
      state.error = "Workspace agent binding could not be saved."
      return false
    } finally {
      state.saving = false
    }
  }

  async function create(): Promise<void> {
    const { form } = state
    let limits: unknown
    try {
      limits = rateLimits(form.rateLimits)
    } catch {
      state.error = "Rate limits must be valid JSON."
      return
    }
    if (
      !(await save(
        `workspace-agents/${encodeURIComponent(workspaceID)}`,
        "POST",
        {
          alias: form.alias,
          model: form.model,
          label: optional(form.label),
          system_prompt: optional(form.systemPrompt),
          prelude: optional(form.prelude),
          rate_limits: limits,
          default: form.default,
          enabled: form.enabled,
        },
      ))
    )
      return
    state.form = {
      alias: "",
      model: "",
      label: "",
      systemPrompt: "",
      prelude: "",
      rateLimits: "",
      default: false,
      enabled: true,
    }
  }

  async function setEnabled(
    binding: SystemWorkspaceAgent,
    enabled: boolean,
  ): Promise<void> {
    await save(
      `workspace-agents/${encodeURIComponent(binding.workspace)}/${encodeURIComponent(binding.id)}`,
      "PATCH",
      {
        model: binding.model,
        label: binding.label,
        system_prompt: binding.system_prompt,
        prelude: binding.prelude,
        rate_limits: binding.rate_limits,
        default: enabled && binding.default,
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
          name: "workspace-agent-bindings",
          topic: "sys",
          events: ["workspace_agent.*"],
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
