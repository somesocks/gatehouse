import type { ActivityClient } from "../../../app/activity"
import {
  searchSystemStorageProviders,
  type SystemStorageProvider,
} from "../../../app/system"
import { createDirectoryController } from "../../../utils/directory-controller.svelte"
export function createStorageProviderListController(options: {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const directory = createDirectoryController<SystemStorageProvider>({
    ...options,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await searchSystemStorageProviders(
        name,
        cursor,
        status,
        signal,
      )
      const result = response.ok
        ? ((await response.json()) as {
            providers: SystemStorageProvider[]
            next_cursor?: string
          })
        : undefined
      return { response, items: result?.providers, next: result?.next_cursor }
    },
  })
  function start() {
    void directory.search()
    const unsubscribe = options.activity.subscribe(
      [
        {
          name: "storage-providers",
          topic: "sys",
          events: ["storage_provider.*"],
        },
      ],
      async () => {
        if (!(await directory.reload()))
          throw new Error("Storage refresh failed")
      },
    )
    return () => {
      unsubscribe()
      directory.stop()
    }
  }
  return { ...directory, start }
}
