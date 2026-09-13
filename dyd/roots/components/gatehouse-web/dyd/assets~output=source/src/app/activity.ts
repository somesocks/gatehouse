import {
  ActivityTopicPoller,
  type ActivityRefresh,
  type ActivitySelector,
  type ActivityTopicCheckpoint,
} from "../utils/activity-poller"

export type { ActivityRefresh, ActivitySelector }
export type ActivityClient = ReturnType<typeof createActivityClient>

type ActivityClientOptions = {
  onAuthenticationLost: () => void
  onPollComplete?: () => void
  fetch?: typeof globalThis.fetch
}

export function createActivityClient({
  onAuthenticationLost,
  onPollComplete,
  fetch: request = globalThis.fetch,
}: ActivityClientOptions) {
  const poller = new ActivityTopicPoller(
    async (topics, signal) => {
      const response = await request("/api/v1/activity", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ topics }),
        signal,
      })
      if (response.status === 401) {
        onAuthenticationLost()
        throw new Error("activity polling requires authentication")
      }
      if (!response.ok) {
        throw new Error("activity checkpoints could not be loaded")
      }
      const output = (await response.json()) as {
        topics: {
          name?: string
          topic: string
          events: string[]
          cursor?: ActivityTopicCheckpoint["cursor"]
        }[]
      }
      return output.topics.map((checkpoint) => {
        if (checkpoint.name === undefined) {
          throw new Error("activity checkpoint name is missing")
        }
        return {
          name: checkpoint.name,
          topic: checkpoint.topic,
          events: checkpoint.events,
          cursor: checkpoint.cursor ?? null,
        }
      })
    },
    1000,
    undefined,
    onPollComplete,
  )

  return {
    subscribe(
      selectors: Iterable<ActivitySelector>,
      refresh: ActivityRefresh,
    ): () => void {
      return poller.subscribe(selectors, refresh)
    },
    async poll(): Promise<void> {
      await poller.poll()
    },
    dispose(): void {
      poller.stop()
    },
  }
}
