import type { ActivityClient } from "../../../app/activity"
import { systemAdministration, type SystemAgentProvider } from "../../../app/system"

export function createAgentProviderListController({ activity, onAuthenticationLost, onSystemAccessChange }: { activity: ActivityClient; onAuthenticationLost: () => void; onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void }) {
  const state = $state({ providers: [] as SystemAgentProvider[], search: "", error: "" })
  let unsubscribe: (() => void) | undefined
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
  function start(): () => void { stop(); unsubscribe = activity.subscribe([{ name: "agent-providers", topic: "sys", events: ["agent_provider.*"] }], async () => await load()); return stop }
  function stop(): void { unsubscribe?.(); unsubscribe = undefined }
  return { state, load, start, stop }
}
