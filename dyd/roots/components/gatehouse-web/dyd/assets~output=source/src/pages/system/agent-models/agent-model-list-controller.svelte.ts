import type { ActivityClient } from "../../../app/activity"
import {
  searchSystemAgentModels,
  type SystemAgentModel,
} from "../../../app/system"
import { createDirectoryController } from "../../../utils/directory-controller.svelte"
export function createAgentModelListController(options: {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const directory = createDirectoryController<SystemAgentModel>({
    ...options,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await searchSystemAgentModels(
        name,
        cursor,
        status,
        signal,
      )
      const result = response.ok
        ? ((await response.json()) as {
            models: SystemAgentModel[]
            next_cursor?: string
          })
        : undefined
      return { response, items: result?.models, next: result?.next_cursor }
    },
  })
  function start() {
    void directory.search()
    const unsubscribe = options.activity.subscribe(
      [{ name: "agent-models", topic: "sys", events: ["agent_model.*"] }],
      async () => {
        await directory.reload()
      },
    )
    return () => {
      unsubscribe()
      directory.stop()
    }
  }
  return { ...directory, start }
}
