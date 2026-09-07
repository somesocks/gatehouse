import type { ActivityClient } from "../../../app/activity"
import { systemAdministration, type SystemAgentModel } from "../../../app/system"

type AgentModelsControllerOptions = {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}

export function createAgentModelsController({ activity, onAuthenticationLost, onSystemAccessChange }: AgentModelsControllerOptions) {
  const state = $state({
    models: [] as SystemAgentModel[],
    error: "",
    saving: false,
    form: { alias: "", provider: "", model: "", parameters: "", compaction: "", maxTurns: 0, maxOutputTokens: 0 },
  })
  let unsubscribe: (() => void) | undefined

  async function load(): Promise<void> {
    state.error = ""
    try {
      const response = await systemAdministration("agent-models")
      if (response.status === 401) { onAuthenticationLost(); return }
      if (response.status === 403) { onSystemAccessChange("denied"); return }
      if (!response.ok) { state.error = "Agent models could not be loaded."; return }
      state.models = await response.json() as SystemAgentModel[]
    } catch { state.error = "Agent models could not be loaded." }
  }

  async function save(path: string, method: "POST" | "PATCH", body: Record<string, unknown>): Promise<boolean> {
    state.error = ""
    state.saving = true
    try {
      const response = await systemAdministration(path, method, body)
      if (response.status === 401) { onAuthenticationLost(); return false }
      if (response.status === 403) { onSystemAccessChange("denied"); return false }
      if (response.status === 409) { state.error = "This model changed elsewhere. The latest settings have been reloaded."; await load(); return false }
      if (!response.ok) { state.error = "Agent model could not be saved."; return false }
      await load()
      return true
    } catch { state.error = "Agent model could not be saved."; return false } finally { state.saving = false }
  }

  async function create(): Promise<void> {
    if (!await save("agent-models", "POST", { alias: state.form.alias, provider: state.form.provider, model: state.form.model, parameters: state.form.parameters, compaction: state.form.compaction, max_turns: state.form.maxTurns, max_output_tokens: state.form.maxOutputTokens, enabled: true })) return
    state.form = { alias: "", provider: "", model: "", parameters: "", compaction: "", maxTurns: 0, maxOutputTokens: 0 }
  }

  async function setEnabled(model: SystemAgentModel, enabled: boolean): Promise<void> {
    await save(`agent-models/${encodeURIComponent(model.id)}`, "PATCH", { alias: model.alias, provider: model.provider, model: model.model, parameters: model.parameters, compaction: model.compaction, max_turns: model.max_turns, max_output_tokens: model.max_output_tokens, enabled, expected_revision: model.revision })
  }

  function start(): () => void {
    stop()
    unsubscribe = activity.subscribe([{ name: "agent-models", topic: "sys", events: ["agent_model.*"] }], async () => await load())
    return stop
  }

  function stop(): void {
    unsubscribe?.()
    unsubscribe = undefined
  }

  return { state, load, create, setEnabled, start, stop }
}
