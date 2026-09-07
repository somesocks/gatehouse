import type { ActivityClient } from "../../app/activity"
import { systemAdministration, type SystemAgentProvider } from "../../app/system"

type AgentProvidersControllerOptions = {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}

export function createAgentProvidersController({ activity, onAuthenticationLost, onSystemAccessChange }: AgentProvidersControllerOptions) {
  const state = $state({
    providers: [] as SystemAgentProvider[],
    error: "",
    saving: false,
    form: { alias: "", protocol: "openai-responses", baseURL: "", keychain: "", apiKey: "" },
  })
  let unsubscribe: (() => void) | undefined

  const optional = (value: string): string | undefined => value.trim() === "" ? undefined : value.trim()

  async function load(): Promise<void> {
    state.error = ""
    try {
      const response = await systemAdministration("agent-providers")
      if (response.status === 401) { onAuthenticationLost(); return }
      if (response.status === 403) { onSystemAccessChange("denied"); return }
      if (!response.ok) { state.error = "Agent providers could not be loaded."; return }
      state.providers = await response.json() as SystemAgentProvider[]
    } catch { state.error = "Agent providers could not be loaded." }
  }

  async function save(path: string, method: "POST" | "PATCH", body: Record<string, unknown>): Promise<boolean> {
    state.error = ""
    state.saving = true
    try {
      const response = await systemAdministration(path, method, body)
      if (response.status === 401) { onAuthenticationLost(); return false }
      if (response.status === 403) { onSystemAccessChange("denied"); return false }
      if (response.status === 409) { state.error = "This provider changed elsewhere. The latest settings have been reloaded."; await load(); return false }
      if (!response.ok) { state.error = "Agent provider could not be saved."; return false }
      await load()
      return true
    } catch { state.error = "Agent provider could not be saved."; return false } finally { state.saving = false }
  }

  async function create(): Promise<void> {
    if (!await save("agent-providers", "POST", { alias: state.form.alias, protocol: state.form.protocol, base_url: optional(state.form.baseURL), keychain: optional(state.form.keychain), api_key: optional(state.form.apiKey), enabled: true })) return
    state.form = { alias: "", protocol: "openai-responses", baseURL: "", keychain: "", apiKey: "" }
  }

  async function setEnabled(provider: SystemAgentProvider, enabled: boolean): Promise<void> {
    await save(`agent-providers/${encodeURIComponent(provider.id)}`, "PATCH", { alias: provider.alias, protocol: provider.protocol, base_url: provider.base_url, keychain: provider.keychain?.id, enabled, expected_revision: provider.revision })
  }

  function start(): () => void {
    stop()
    unsubscribe = activity.subscribe([{ name: "agent-providers", topic: "sys", events: ["agent_provider.*"] }], async () => await load())
    return stop
  }

  function stop(): void {
    unsubscribe?.()
    unsubscribe = undefined
  }

  return { state, load, create, setEnabled, start, stop }
}
