import type { ActivityClient } from "../../../app/activity"
import {
  searchSystemAgentProviders,
  type SystemAgentProvider,
} from "../../../app/system"
import { createDirectoryController } from "../../../utils/directory-controller.svelte"
export function createAgentProviderListController(options: {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const directory = createDirectoryController<SystemAgentProvider>({
    ...options,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await searchSystemAgentProviders(
        name,
        cursor,
        status,
        signal,
      )
      const result = response.ok
        ? ((await response.json()) as {
            providers: SystemAgentProvider[]
            next_cursor?: string
          })
        : undefined
      return { response, items: result?.providers, next: result?.next_cursor }
    },
  })
  function start() {
    void directory.search()
    const unsubscribe = options.activity.subscribe(
      [{ name: "agent-providers", topic: "sys", events: ["agent_provider.*"] }],
      async () => {
        if (!(await directory.reload()))
          throw new Error("Provider refresh failed")
      },
    )
    return () => {
      unsubscribe()
      directory.stop()
    }
  }
  return { ...directory, start }
}
