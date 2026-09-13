import {
  systemAdministration,
  type SystemAgentModel,
  type SystemAgentProvider,
} from "../../../app/system"
type Form = {
  alias: string
  provider: string
  model: string
  parameters: string
  compaction: string
  maxTurns: number
  maxOutputTokens: number
  enabled: boolean
}
const blank = (): Form => ({
  alias: "",
  provider: "",
  model: "",
  parameters: "",
  compaction: "",
  maxTurns: 0,
  maxOutputTokens: 0,
  enabled: true,
})
export function createAgentModelFormController({
  onAuthenticationLost,
  onSystemAccessChange,
}: {
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const state = $state({
    model: null as SystemAgentModel | null,
    providers: [] as SystemAgentProvider[],
    form: blank(),
    error: "",
    loading: false,
    saving: false,
    editing: false,
  })
  async function loadProviders(): Promise<void> {
    const response = await systemAdministration("agent-providers")
    if (response.status === 401) {
      onAuthenticationLost()
      return
    }
    if (response.status === 403) {
      onSystemAccessChange("denied")
      return
    }
    if (response.ok)
      state.providers = (await response.json()) as SystemAgentProvider[]
  }
  async function load(id: string): Promise<void> {
    state.error = ""
    state.loading = true
    try {
      const response = await systemAdministration(
        `agent-models/${encodeURIComponent(id)}`,
      )
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return
      }
      if (response.status === 404) {
        state.error = "Agent model was not found."
        return
      }
      if (!response.ok) {
        state.error = "Agent model could not be loaded."
        return
      }
      state.model = (await response.json()) as SystemAgentModel
    } catch {
      state.error = "Agent model could not be loaded."
    } finally {
      state.loading = false
    }
  }
  async function beginEdit(): Promise<void> {
    if (state.model === null) return
    state.form = {
      alias: state.model.alias,
      provider: state.model.provider,
      model: state.model.model,
      parameters: state.model.parameters,
      compaction: state.model.compaction,
      maxTurns: state.model.max_turns,
      maxOutputTokens: state.model.max_output_tokens,
      enabled: state.model.enabled,
    }
    await loadProviders()
    state.editing = true
  }
  function valid(): boolean {
    if (state.form.provider === "") {
      state.error = "A provider is required."
      return false
    }
    return true
  }
  async function create(): Promise<SystemAgentModel | null> {
    state.error = ""
    if (!valid()) return null
    return await save("agent-models", "POST", state.form)
  }
  async function update(): Promise<boolean> {
    if (state.model === null) return false
    state.error = ""
    if (!valid()) return false
    const saved = await save(
      `agent-models/${encodeURIComponent(state.model.id)}`,
      "PATCH",
      state.form,
      state.model.revision,
    )
    if (saved === null) return false
    state.model = saved
    state.editing = false
    return true
  }
  async function save(
    path: string,
    method: "POST" | "PATCH",
    form: Form,
    revision?: number,
  ): Promise<SystemAgentModel | null> {
    state.saving = true
    try {
      const response = await systemAdministration(path, method, {
        alias: form.alias,
        provider: form.provider,
        model: form.model,
        parameters: form.parameters,
        compaction: form.compaction,
        max_turns: form.maxTurns,
        max_output_tokens: form.maxOutputTokens,
        enabled: form.enabled,
        ...(revision === undefined ? {} : { expected_revision: revision }),
      })
      if (response.status === 401) {
        onAuthenticationLost()
        return null
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return null
      }
      if (response.status === 409) {
        state.error =
          "This model changed elsewhere. The latest settings have been reloaded."
        if (state.model !== null) await load(state.model.id)
        return null
      }
      if (!response.ok) {
        state.error = "Agent model could not be saved."
        return null
      }
      return (await response.json()) as SystemAgentModel
    } catch {
      state.error = "Agent model could not be saved."
      return null
    } finally {
      state.saving = false
    }
  }
  return { state, loadProviders, load, beginEdit, create, update }
}
