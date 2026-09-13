import {
  systemAdministration,
  type SystemWorkspaceAgent,
} from "../../../app/system"

const optional = (value: string) =>
  value.trim() === "" ? undefined : value.trim()

export function createWorkspaceAgentBindingController({
  onAuthenticationLost,
  onSystemAccessChange,
}: {
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const state = $state({
    binding: null as SystemWorkspaceAgent | null,
    form: {
      workspace: "",
      alias: "",
      model: "",
      priority: 0,
      label: "",
      systemPrompt: "",
      enabled: true,
    },
    error: "",
    loading: false,
    saving: false,
    editing: false,
  })

  async function load(workspace: string, bindingID: string) {
    state.loading = true
    state.error = ""
    try {
      const response = await systemAdministration(
        `workspace-agents/${encodeURIComponent(workspace)}/${encodeURIComponent(bindingID)}`,
      )
      if (response.status === 401) onAuthenticationLost()
      else if (response.status === 403) onSystemAccessChange("denied")
      else if (response.status === 404)
        state.error = "Workspace agent binding was not found."
      else if (!response.ok)
        state.error = "Workspace agent binding could not be loaded."
      else state.binding = (await response.json()) as SystemWorkspaceAgent
    } catch {
      state.error = "Workspace agent binding could not be loaded."
    } finally {
      state.loading = false
    }
  }

  function edit() {
    if (!state.binding) return
    state.form = {
      workspace: state.binding.workspace,
      alias: state.binding.alias,
      model: state.binding.model,
      priority: state.binding.priority,
      label: state.binding.label ?? "",
      systemPrompt: state.binding.system_prompt ?? "",
      enabled: state.binding.enabled,
    }
    state.editing = true
  }

  async function update() {
    if (!state.binding) return
    state.saving = true
    try {
      const form = state.form
      const response = await systemAdministration(
        `workspace-agents/${encodeURIComponent(state.binding.workspace)}/${encodeURIComponent(state.binding.id)}`,
        "PATCH",
        {
          model: form.model,
          priority: form.priority,
          label: optional(form.label),
          system_prompt: optional(form.systemPrompt),
          enabled: form.enabled,
          expected_revision: state.binding.revision,
        },
      )
      if (response.status === 401) onAuthenticationLost()
      else if (response.status === 403) onSystemAccessChange("denied")
      else if (response.status === 409) {
        state.error =
          "This binding changed elsewhere. The latest settings have been reloaded."
        await load(state.binding.workspace, state.binding.id)
      } else if (!response.ok)
        state.error = "Workspace agent binding could not be saved."
      else {
        state.binding = (await response.json()) as SystemWorkspaceAgent
        state.editing = false
      }
    } catch {
      state.error = "Workspace agent binding could not be saved."
    } finally {
      state.saving = false
    }
  }

  return { state, load, edit, update }
}
