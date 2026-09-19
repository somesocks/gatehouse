import type { ActivityClient } from "../../../app/activity"
import {
  systemAdministration,
  type SystemWorkspaceAgent,
} from "../../../app/system"

type WorkspaceAgentBindingsControllerOptions = {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}

export function createWorkspaceAgentBindingsController({
  activity,
  onAuthenticationLost,
  onSystemAccessChange,
}: WorkspaceAgentBindingsControllerOptions) {
  const state = $state({
    bindings: [] as SystemWorkspaceAgent[],
    error: "",
    saving: false,
    form: {
		workspace: "",
		alias: "",
		model: "",
		label: "",
		systemPrompt: "",
		prelude: "",
		default: false,
    },
  })
  let unsubscribe: (() => void) | undefined

  const optional = (value: string): string | undefined =>
    value.trim() === "" ? undefined : value.trim()

  async function load(): Promise<void> {
    state.error = ""
    try {
      const response = await systemAdministration("workspace-agents")
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
      state.bindings = (await response.json()) as SystemWorkspaceAgent[]
    } catch {
      state.error = "Workspace agent bindings could not be loaded."
    }
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
    if (
      !(await save(
        `workspace-agents/${encodeURIComponent(form.workspace)}`,
        "POST",
        {
          alias: form.alias,
          model: form.model,
			label: optional(form.label),
			system_prompt: optional(form.systemPrompt),
			prelude: optional(form.prelude),
			default: form.default,
          enabled: true,
        },
      ))
    )
      return
    state.form = {
      workspace: "",
      alias: "",
      model: "",
		label: "",
		systemPrompt: "",
		prelude: "",
		default: false,
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

  return { state, load, create, setEnabled, start, stop }
}
