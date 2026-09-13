export type ActivityCursor = { id: string }

export type ActivityTopicCheckpoint = {
  name: string
  topic: string
  events: string[]
  cursor: ActivityCursor | null
}

export type ActivitySelector = Pick<
  ActivityTopicCheckpoint,
  "name" | "topic" | "events"
>

export type ActivityRefresh = (input: {
  names: ReadonlySet<string>
  signal: AbortSignal
}) => Promise<void>

type ActivityRequest = (
  topics: ActivityTopicCheckpoint[],
  signal: AbortSignal,
) => Promise<ActivityTopicCheckpoint[]>

type Scheduler = {
  set: (callback: () => void, delay: number) => ReturnType<typeof setTimeout>
  clear: (timer: ReturnType<typeof setTimeout>) => void
}

type Subscription = {
  selectors: Map<string, ActivitySelector>
  refresh: ActivityRefresh
  controller: AbortController | null
}

const defaultScheduler: Scheduler = {
  set: (callback, delay) => setTimeout(callback, delay),
  clear: (timer) => clearTimeout(timer),
}

function sameCursor(
  left: ActivityCursor | null,
  right: ActivityCursor | null,
): boolean {
  return left?.id === right?.id
}

export function activitySelectorKey(selector: ActivitySelector): string {
  return `${selector.topic}\u0000${[...new Set(selector.events)].sort().join("\u0000")}`
}

function normalizeSelector(selector: ActivitySelector): ActivitySelector {
  return {
    name: selector.name,
    topic: selector.topic,
    events: [...new Set(selector.events)].sort(),
  }
}

export class ActivityTopicPoller {
  #checkpoints = new Map<string, ActivityCursor | null>()
  #subscriptions = new Set<Subscription>()
  #requestController: AbortController | null = null
  #timer: ReturnType<typeof setTimeout> | null = null

  constructor(
    private readonly request: ActivityRequest,
    private readonly interval = 1000,
    private readonly scheduler: Scheduler = defaultScheduler,
    private readonly onPollComplete?: () => void,
  ) {}

  subscribe(
    selectors: Iterable<ActivitySelector>,
    refresh: ActivityRefresh,
  ): () => void {
    const normalized = new Map<string, ActivitySelector>()
    for (const selector of selectors) {
      const value = normalizeSelector(selector)
      if (normalized.has(value.name)) {
        throw new Error(
          `duplicate activity subscription name ${JSON.stringify(value.name)}`,
        )
      }
      normalized.set(value.name, value)
    }
    for (const existing of this.#subscriptions) {
      for (const name of normalized.keys()) {
        if (existing.selectors.has(name)) {
          throw new Error(
            `duplicate activity subscription name ${JSON.stringify(name)}`,
          )
        }
      }
    }
    const subscription: Subscription = {
      selectors: normalized,
      refresh,
      controller: null,
    }
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
  }

  async poll(): Promise<void> {
    if (this.#requestController !== null || this.#subscriptions.size === 0) {
      return
    }
    const selectors = new Map<
      string,
      { selector: ActivitySelector; names: Set<string> }
    >()
    for (const subscription of this.#subscriptions) {
      for (const [name, selector] of subscription.selectors) {
        const key = activitySelectorKey(selector)
        const existing = selectors.get(key)
        if (existing === undefined) {
          selectors.set(key, { selector, names: new Set([name]) })
        } else {
          existing.names.add(name)
        }
      }
    }
    if (selectors.size === 0) {
      return
    }

    const subscriptions = [...this.#subscriptions]
    const controller = new AbortController()
    this.#requestController = controller
    const input = [...selectors].map(([key, { selector }]) => ({
      ...selector,
      cursor: this.#checkpoints.get(key) ?? null,
    }))
    try {
      const output = await this.request(input, controller.signal)
      if (controller.signal.aborted) {
        return
      }
      const inputKeysByName = new Map(
        input.map((checkpoint) => [
          checkpoint.name,
          activitySelectorKey(checkpoint),
        ]),
      )
      const next = new Map<string, ActivityCursor | null>()
      for (const checkpoint of output) {
        const key = inputKeysByName.get(checkpoint.name)
        if (key !== undefined) {
          next.set(key, checkpoint.cursor)
        }
      }
      const dispatches: { subscription: Subscription; names: Set<string> }[] =
        []
      for (const subscription of subscriptions) {
        const changed = new Set(
          [...subscription.selectors]
            .filter(([, selector]) => {
              const key = activitySelectorKey(selector)
              return (
                this.#checkpoints.has(key) &&
                !sameCursor(
                  this.#checkpoints.get(key) ?? null,
                  next.has(key)
                    ? (next.get(key) ?? null)
                    : (this.#checkpoints.get(key) ?? null),
                )
              )
            })
            .map(([name]) => name),
        )
        if (changed.size > 0) {
          dispatches.push({ subscription, names: changed })
        }
      }

      const failedSelectors = new Set<string>()
      await Promise.all(
        dispatches.map(async (dispatch) => {
          if (!this.#subscriptions.has(dispatch.subscription)) {
            for (const name of dispatch.names) {
              failedSelectors.add(
                activitySelectorKey(dispatch.subscription.selectors.get(name)!),
              )
            }
            return
          }
          const refreshController = new AbortController()
          dispatch.subscription.controller?.abort()
          dispatch.subscription.controller = refreshController
          try {
            await dispatch.subscription.refresh({
              names: dispatch.names,
              signal: refreshController.signal,
            })
            if (
              refreshController.signal.aborted ||
              !this.#subscriptions.has(dispatch.subscription)
            ) {
              for (const name of dispatch.names) {
                failedSelectors.add(
                  activitySelectorKey(
                    dispatch.subscription.selectors.get(name)!,
                  ),
                )
              }
              return
            }
          } catch {
            for (const name of dispatch.names) {
              failedSelectors.add(
                activitySelectorKey(dispatch.subscription.selectors.get(name)!),
              )
            }
          } finally {
            if (dispatch.subscription.controller === refreshController) {
              dispatch.subscription.controller = null
            }
          }
        }),
      )

      for (const key of selectors.keys()) {
        if (!failedSelectors.has(key)) {
          this.#checkpoints.set(
            key,
            next.has(key)
              ? (next.get(key) ?? null)
              : (this.#checkpoints.get(key) ?? null),
          )
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
      if (this.#subscriptions.size > 0) {
        this.schedule(this.interval)
      }
    }
  }

  private schedule(delay: number): void {
    if (this.#subscriptions.size === 0) {
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
