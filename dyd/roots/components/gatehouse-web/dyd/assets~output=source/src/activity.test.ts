import { describe, expect, it } from "vitest"
import { ActivityTopicPoller } from "./activity"

const scheduler = {
  set: () => 0 as ReturnType<typeof setTimeout>,
  clear: () => {},
}

function createScheduler() {
  let nextTimer = 0
  const timers = new Map<number, { callback: () => void; delay: number }>()
  const scheduledDelays: number[] = []
  return {
    scheduler: {
      set(callback: () => void, delay: number) {
        const timer = ++nextTimer
        timers.set(timer, { callback, delay })
        scheduledDelays.push(delay)
        return timer as ReturnType<typeof setTimeout>
      },
      clear(timer: ReturnType<typeof setTimeout>) {
        timers.delete(timer as unknown as number)
      },
    },
    scheduledDelays,
    pendingDelays: () => [...timers.values()].map((timer) => timer.delay),
    runNext() {
      const next = timers.entries().next().value as [number, { callback: () => void; delay: number }] | undefined
      if (next === undefined) {
        throw new Error("No scheduled poll")
      }
      const [timer, { callback }] = next
      timers.delete(timer)
      callback()
    },
  }
}

describe("activity topic poller", () => {
  it("tracks independent checkpoints and refreshes only changed subscriptions", async () => {
    const requests: { topic: string; cursor: string | null }[][] = []
    const responses = [
      [{ topic: "session/ses_a", cursor: { id: "act_1" } }, { topic: "project/prj_a", cursor: { id: "act_2" } }],
      [{ topic: "session/ses_a", cursor: { id: "act_3" } }, { topic: "project/prj_a", cursor: { id: "act_2" } }],
    ]
    const poller = new ActivityTopicPoller(async (_workspaceID, topics) => {
      requests.push(topics.map((topic) => ({ topic: topic.topic, cursor: topic.cursor?.id ?? null })))
      return responses.shift() ?? []
    }, 1000, scheduler)
    const refreshes: { projection: string; topics: string[] }[] = []

    poller.setWorkspace("wsp_a")
    poller.subscribe(["session/ses_a"], async ({ topics }) => {
      refreshes.push({ projection: "session", topics: [...topics] })
    })
    poller.subscribe(["project/prj_a"], async ({ topics }) => {
      refreshes.push({ projection: "project", topics: [...topics] })
    })

    await poller.poll()
    await poller.poll()

    expect(requests).toEqual([
      [{ topic: "session/ses_a", cursor: null }, { topic: "project/prj_a", cursor: null }],
      [{ topic: "session/ses_a", cursor: "act_1" }, { topic: "project/prj_a", cursor: "act_2" }],
    ])
    expect(refreshes).toEqual([
      { projection: "session", topics: ["session/ses_a"] },
    ])
  })

  it("retries a topic without advancing its checkpoint after refresh failure", async () => {
    const requests: (string | null)[] = []
    let attempts = 0
    const poller = new ActivityTopicPoller(async (_workspaceID, topics) => {
      requests.push(topics[0].cursor?.id ?? null)
      return [{ topic: "session/ses_a", cursor: { id: requests.length === 1 ? "act_1" : "act_2" } }]
    }, 1000, scheduler)

    poller.setWorkspace("wsp_a")
    poller.subscribe(["session/ses_a"], async () => {
      attempts += 1
      if (attempts === 1) {
        throw new Error("temporary failure")
      }
    })

    await poller.poll()
    await poller.poll()
    await poller.poll()

    expect(requests).toEqual([null, "act_1", "act_1"])
    poller.stop()
  })

  it("does not acknowledge a topic when its subscription is aborted", async () => {
    const requests: (string | null)[] = []
    let startRefresh: (() => void) | undefined
    let finishRefresh: (() => void) | undefined
    const refreshStarted = new Promise<void>((resolve) => startRefresh = resolve)
    const refreshFinished = new Promise<void>((resolve) => finishRefresh = resolve)
    let responseID = 0
    const poller = new ActivityTopicPoller(async (_workspaceID, topics) => {
      requests.push(topics[0].cursor?.id ?? null)
      responseID += 1
      return [{ topic: "session/ses_a", cursor: { id: `act_${responseID}` } }]
    }, 1000, scheduler)

    poller.setWorkspace("wsp_a")
    const unsubscribe = poller.subscribe(["session/ses_a"], async () => {
      startRefresh?.()
      await refreshFinished
    })
    await poller.poll()
    const firstPoll = poller.poll()
    await refreshStarted
    unsubscribe()
    finishRefresh?.()
    await firstPoll
    poller.subscribe(["session/ses_a"], async () => {})
    await poller.poll()

    expect(requests).toEqual([null, "act_1", "act_1"])
    poller.stop()
  })

  it("polls automatically and schedules the next poll", async () => {
    const testScheduler = createScheduler()
    let complete: (() => void) | undefined
    const completed = new Promise<void>((resolve) => complete = resolve)
    const requests: string[] = []
    const poller = new ActivityTopicPoller(async (workspaceID) => {
      requests.push(workspaceID)
      return []
    }, 1000, testScheduler.scheduler, () => complete?.())

    poller.setWorkspace("wsp_a")
    poller.subscribe(["session/ses_a"], async () => {})
    testScheduler.runNext()
    await completed

    expect(requests).toEqual(["wsp_a"])
    expect(testScheduler.scheduledDelays).toEqual([0, 1000])
    expect(testScheduler.pendingDelays()).toEqual([1000])
    poller.stop()
  })

  it("does not acknowledge a topic after unsubscribing before the request resolves", async () => {
    const requests: (string | null)[] = []
    let resolveRequest: ((checkpoints: { topic: string; cursor: { id: string } }[]) => void) | undefined
    const request = new Promise<{ topic: string; cursor: { id: string } }[]>((resolve) => resolveRequest = resolve)
    let requestCount = 0
    const poller = new ActivityTopicPoller(async (_workspaceID, topics) => {
      requests.push(topics[0].cursor?.id ?? null)
      requestCount += 1
      return requestCount === 1 ? [{ topic: "session/ses_a", cursor: { id: "act_1" } }] : request
    }, 1000, scheduler)

    poller.setWorkspace("wsp_a")
    const unsubscribe = poller.subscribe(["session/ses_a"], async () => {})
    await poller.poll()
    const firstPoll = poller.poll()
    unsubscribe()
    resolveRequest?.([{ topic: "session/ses_a", cursor: { id: "act_2" } }])
    await firstPoll
    poller.subscribe(["session/ses_a"], async () => {})
    await poller.poll()

    expect(requests).toEqual([null, "act_1", "act_1"])
    poller.stop()
  })

  it("restarts polling before an aborted request settles", async () => {
    const testScheduler = createScheduler()
    const requests: string[] = []
    const resolvers: (() => void)[] = []
    let complete: (() => void) | undefined
    const completed = new Promise<void>((resolve) => complete = resolve)
    const poller = new ActivityTopicPoller((workspaceID) => new Promise<void>((resolve) => {
      requests.push(workspaceID)
      resolvers.push(resolve)
    }).then(() => []), 1000, testScheduler.scheduler, () => complete?.())

    poller.setWorkspace("wsp_a")
    poller.subscribe(["session/ses_a"], async () => {})
    const firstPoll = poller.poll()
    poller.stop()
    poller.setWorkspace("wsp_b")
    poller.subscribe(["session/ses_b"], async () => {})
    testScheduler.runNext()

    expect(requests).toEqual(["wsp_a", "wsp_b"])
    resolvers[1]?.()
    await completed
    resolvers[0]?.()
    await firstPoll
    poller.stop()
  })
})
