export type ActivityCursor = { id: string }

export type ActivityTopicCheckpoint = {
  topic: string
  cursor: ActivityCursor | null
}

export type ActivityRefresh = (input: { topics: ReadonlySet<string>; signal: AbortSignal }) => Promise<void>

type ActivityRequest = (workspaceID: string, topics: ActivityTopicCheckpoint[], signal: AbortSignal) => Promise<ActivityTopicCheckpoint[]>

type Scheduler = {
  set: (callback: () => void, delay: number) => ReturnType<typeof setTimeout>
  clear: (timer: ReturnType<typeof setTimeout>) => void
}

type Subscription = {
  topics: Set<string>
  refresh: ActivityRefresh
  controller: AbortController | null
}

const defaultScheduler: Scheduler = {
  set: (callback, delay) => setTimeout(callback, delay),
  clear: (timer) => clearTimeout(timer),
}

function sameCursor(left: ActivityCursor | null, right: ActivityCursor | null): boolean {
  return left?.id === right?.id
}

export class ActivityTopicPoller {
  #workspaceID: string | null = null
  #checkpoints = new Map<string, ActivityCursor | null>()
  #subscriptions = new Set<Subscription>()
  #requestController: AbortController | null = null
  #timer: ReturnType<typeof setTimeout> | null = null

  constructor(private readonly request: ActivityRequest, private readonly interval = 1000, private readonly scheduler: Scheduler = defaultScheduler, private readonly onPollComplete?: () => void) {}

  setWorkspace(workspaceID: string | null): void {
    if (this.#workspaceID === workspaceID) {
      return
    }
    this.#workspaceID = workspaceID
    this.#checkpoints.clear()
    this.#requestController?.abort()
    this.#requestController = null
    for (const subscription of this.#subscriptions) {
      subscription.controller?.abort()
    }
    this.schedule(0)
  }

  subscribe(topics: Iterable<string>, refresh: ActivityRefresh): () => void {
    const subscription: Subscription = { topics: new Set(topics), refresh, controller: null }
    this.#subscriptions.add(subscription)
    this.schedule(0)
    return () => {
      subscription.controller?.abort()
      this.#subscriptions.delete(subscription)
      if (this.#subscriptions.size === 0) {
        this.clearTimer()
      }
    }
  }

  stop(): void {
    this.clearTimer()
    this.#requestController?.abort()
    this.#requestController = null
    for (const subscription of this.#subscriptions) {
      subscription.controller?.abort()
    }
    this.#subscriptions.clear()
    this.#checkpoints.clear()
    this.#workspaceID = null
  }

  async poll(): Promise<void> {
    if (this.#requestController !== null || this.#workspaceID === null || this.#subscriptions.size === 0) {
      return
    }
    const workspaceID = this.#workspaceID
    const topics = new Set<string>()
    for (const subscription of this.#subscriptions) {
      for (const topic of subscription.topics) {
        topics.add(topic)
      }
    }
    if (topics.size === 0) {
      return
    }

    const subscriptions = [...this.#subscriptions]
    const controller = new AbortController()
    this.#requestController = controller
    const input = [...topics].map((topic) => ({ topic, cursor: this.#checkpoints.get(topic) ?? null }))
    try {
      const output = await this.request(workspaceID, input, controller.signal)
      if (controller.signal.aborted || this.#workspaceID !== workspaceID) {
        return
      }
      const next = new Map(output.map((checkpoint) => [checkpoint.topic, checkpoint.cursor]))
      const dispatches: { subscription: Subscription; topics: Set<string> }[] = []
      for (const subscription of subscriptions) {
        const changed = new Set([...subscription.topics].filter((topic) => this.#checkpoints.has(topic) && !sameCursor(this.#checkpoints.get(topic) ?? null, next.has(topic) ? next.get(topic) ?? null : this.#checkpoints.get(topic) ?? null)))
        if (changed.size > 0) {
          dispatches.push({ subscription, topics: changed })
        }
      }

      const failedTopics = new Set<string>()
      await Promise.all(dispatches.map(async (dispatch) => {
        if (!this.#subscriptions.has(dispatch.subscription)) {
          for (const topic of dispatch.topics) {
            failedTopics.add(topic)
          }
          return
        }
        const refreshController = new AbortController()
        dispatch.subscription.controller?.abort()
        dispatch.subscription.controller = refreshController
        try {
          await dispatch.subscription.refresh({ topics: dispatch.topics, signal: refreshController.signal })
          if (refreshController.signal.aborted || !this.#subscriptions.has(dispatch.subscription)) {
            for (const topic of dispatch.topics) {
              failedTopics.add(topic)
            }
            return
          }
        } catch {
          for (const topic of dispatch.topics) {
            failedTopics.add(topic)
          }
        } finally {
          if (dispatch.subscription.controller === refreshController) {
            dispatch.subscription.controller = null
          }
        }
      }))

      if (this.#workspaceID !== workspaceID) {
        return
      }
      for (const topic of topics) {
        if (!failedTopics.has(topic)) {
          this.#checkpoints.set(topic, next.has(topic) ? next.get(topic) ?? null : this.#checkpoints.get(topic) ?? null)
        }
      }
    } catch {
      // Keep the current checkpoints so the next poll retries the same changes.
    } finally {
      if (this.#requestController !== controller) {
        return
      }
      this.#requestController = null
      this.onPollComplete?.()
      if (this.#workspaceID !== null && this.#subscriptions.size > 0) {
        this.schedule(this.#workspaceID === workspaceID ? this.interval : 0)
      }
    }
  }

  private schedule(delay: number): void {
    if (this.#workspaceID === null || this.#subscriptions.size === 0) {
      return
    }
    this.clearTimer()
    this.#timer = this.scheduler.set(() => {
      this.#timer = null
      void this.poll()
    }, delay)
  }

  private clearTimer(): void {
    if (this.#timer !== null) {
      this.scheduler.clear(this.#timer)
      this.#timer = null
    }
  }
}
