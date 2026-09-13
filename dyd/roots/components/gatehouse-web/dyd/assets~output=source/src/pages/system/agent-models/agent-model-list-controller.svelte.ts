import type { ActivityClient } from "../../../app/activity"
import {
  systemAdministration,
  type SystemAgentModel,
} from "../../../app/system"
export function createAgentModelListController({
  activity,
  onAuthenticationLost,
  onSystemAccessChange,
}: {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const state = $state({
    models: [] as SystemAgentModel[],
    search: "",
    error: "",
  })
  let unsubscribe: (() => void) | undefined
  async function load(): Promise<void> {
    state.error = ""
    try {
      const response = await systemAdministration("agent-models")
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return
      }
      if (!response.ok) {
        state.error = "Agent models could not be loaded."
        return
      }
      state.models = (await response.json()) as SystemAgentModel[]
    } catch {
      state.error = "Agent models could not be loaded."
    }
  }
  function start(): () => void {
    stop()
    unsubscribe = activity.subscribe(
      [{ name: "agent-models", topic: "sys", events: ["agent_model.*"] }],
      async () => await load(),
    )
    return stop
  }
  function stop(): void {
    unsubscribe?.()
    unsubscribe = undefined
  }
  return { state, load, start, stop }
}
