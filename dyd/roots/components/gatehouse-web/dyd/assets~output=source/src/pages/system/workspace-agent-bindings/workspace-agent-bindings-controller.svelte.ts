import type { ActivityClient } from "../../../app/activity"
import {
  fetchSystemWorkspaceAgents,
  systemAdministration,
  type SystemWorkspaceAgent,
} from "../../../app/system"
import { createLoadState } from "../../../utils/load-state.svelte"

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
  const resource = createLoadState()
  const mutation = createLoadState()
  let appliedSearch = ""
  const state = $state({
    bindings: [] as SystemWorkspaceAgent[],
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

  async function load(): Promise<boolean> {
    const request = resource.begin()
    const search = state.search
    state.error = ""
    try {
      const response = await fetchSystemWorkspaceAgents(
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
        state.error = "Workspace agent bindings could not be loaded."
        request.fail(new Error(state.error))
        return false
      }
      const result = (await response.json()) as {
        agents: SystemWorkspaceAgent[]
        next_cursor?: string
      }
      return request.commit(() => {
        state.bindings = result.agents
        state.cursor = result.next_cursor ?? ""
        appliedSearch = search
      })
    } catch {
      if (request.current()) {
        state.error = "Workspace agent bindings could not be loaded."
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
      const response = await fetchSystemWorkspaceAgents(
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
        state.error = "Workspace agent bindings could not be loaded."
        return
      }
      const result = (await response.json()) as {
        agents: SystemWorkspaceAgent[]
        next_cursor?: string
      }
      if (!request.current()) return
      const seen = new Set(state.bindings.map((binding) => binding.id))
      state.bindings = [
        ...state.bindings,
        ...result.agents.filter((binding) => !seen.has(binding.id)),
      ]
      state.cursor = result.next_cursor ?? ""
    } catch {
      if (request.current())
        state.error = "Workspace agent bindings could not be loaded."
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
        state.error = "Workspace agent binding could not be saved."
        return false
      }
      resource.stop()
      if (method === "PATCH") await load()
      return request.current()
    } catch {
      if (request.current())
        state.error = "Workspace agent binding could not be saved."
      return false
    } finally {
      request.finish()
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
      async () => {
        if (!(await load())) throw new Error("Agent binding refresh failed")
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
