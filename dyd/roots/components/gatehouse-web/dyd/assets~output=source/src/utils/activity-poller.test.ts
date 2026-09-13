import { describe, expect, it } from "vitest"
import {
  ActivityTopicPoller,
  type ActivitySelector,
  type ActivityTopicCheckpoint,
} from "./activity-poller"

const scheduler = {
  set: () => 0 as ReturnType<typeof setTimeout>,
  clear: () => {},
}

const session: ActivitySelector = {
  name: "session",
  topic: "wsp_a/ses_a",
  events: ["session.create"],
}
const sessionEvents: ActivitySelector = {
  name: "session-events",
  topic: "wsp_a/ses_a",
  events: ["session_event.create"],
}
const project: ActivitySelector = {
  name: "project",
  topic: "wsp_a/prj_a",
  events: ["project.create"],
}

function checkpoint(
  selector: ActivitySelector,
  id: string | null,
): ActivityTopicCheckpoint {
  return { ...selector, cursor: id === null ? null : { id } }
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
      const next = timers.entries().next().value as
        | [number, { callback: () => void; delay: number }]
        | undefined
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
  it("tracks selector pairs independently and refreshes only changed subscriptions", async () => {
    const requests: ActivityTopicCheckpoint[][] = []
    const responses = [
      [
        checkpoint(session, "act_1"),
        checkpoint(sessionEvents, "act_2"),
        checkpoint(project, "act_3"),
      ],
      [
        checkpoint(session, "act_4"),
        checkpoint(sessionEvents, "act_2"),
        checkpoint(project, "act_3"),
      ],
    ]
    const poller = new ActivityTopicPoller(
      async (selectors) => {
        requests.push(selectors)
        return responses.shift() ?? []
      },
      1000,
      scheduler,
    )
    const refreshes: string[][] = []

    poller.subscribe([session, sessionEvents], async ({ names }) =>
      refreshes.push([...names]),
    )
    poller.subscribe([project], async ({ names }) => refreshes.push([...names]))

    await poller.poll()
    await poller.poll()

    expect(requests).toEqual([
      [
        checkpoint(session, null),
        checkpoint(sessionEvents, null),
        checkpoint(project, null),
      ],
      [
        checkpoint(session, "act_1"),
        checkpoint(sessionEvents, "act_2"),
        checkpoint(project, "act_3"),
      ],
    ])
    expect(refreshes).toEqual([["session"]])
  })

  it("correlates wildcard selectors by name after server normalization", async () => {
    const wildcard: ActivitySelector = {
      name: "sessions",
      topic: "wsp_a",
      events: ["session.*"],
    }
    const requests: ActivityTopicCheckpoint[][] = []
    const responses = [
      [
        {
          ...wildcard,
          events: ["session.create", "session.update"],
          cursor: { id: "act_1" },
        },
      ],
      [
        {
          ...wildcard,
          events: ["session.create", "session.update"],
          cursor: { id: "act_2" },
        },
      ],
    ]
    const refreshes: string[][] = []
    const poller = new ActivityTopicPoller(
      async (selectors) => {
        requests.push(selectors)
        return responses.shift() ?? []
      },
      1000,
      scheduler,
    )

    poller.subscribe([wildcard], async ({ names }) =>
      refreshes.push([...names]),
    )
    await poller.poll()
    await poller.poll()

    expect(requests).toEqual([
      [checkpoint(wildcard, null)],
      [checkpoint(wildcard, "act_1")],
    ])
    expect(refreshes).toEqual([["sessions"]])
  })

  it("retries a selector without advancing its checkpoint after refresh failure", async () => {
    const requests: (string | null)[] = []
    let attempts = 0
    const poller = new ActivityTopicPoller(
      async (selectors) => {
        requests.push(selectors[0]?.cursor?.id ?? null)
        return [checkpoint(session, requests.length === 1 ? "act_1" : "act_2")]
      },
      1000,
      scheduler,
    )

    poller.subscribe([session], async () => {
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

  it("does not acknowledge a selector after unsubscribing before its request resolves", async () => {
    const requests: (string | null)[] = []
    let resolveRequest:
      | ((checkpoints: ActivityTopicCheckpoint[]) => void)
      | undefined
    const request = new Promise<ActivityTopicCheckpoint[]>(
      (resolve) => (resolveRequest = resolve),
    )
    let requestCount = 0
    const poller = new ActivityTopicPoller(
      async (selectors) => {
        requests.push(selectors[0]?.cursor?.id ?? null)
        requestCount += 1
        return requestCount === 1 ? [checkpoint(session, "act_1")] : request
      },
      1000,
      scheduler,
    )

    const unsubscribe = poller.subscribe([session], async () => {})
    await poller.poll()
    const firstPoll = poller.poll()
    unsubscribe()
    resolveRequest?.([checkpoint(session, "act_2")])
    await firstPoll
    poller.subscribe([session], async () => {})
    await poller.poll()

    expect(requests).toEqual([null, "act_1", "act_1"])
    poller.stop()
  })

  it("polls automatically and schedules the next poll", async () => {
    const testScheduler = createScheduler()
    let complete: (() => void) | undefined
    const completed = new Promise<void>((resolve) => (complete = resolve))
    const requests: ActivityTopicCheckpoint[][] = []
    const poller = new ActivityTopicPoller(
      async (selectors) => {
        requests.push(selectors)
        return []
      },
      1000,
      testScheduler.scheduler,
      () => complete?.(),
    )

    poller.subscribe([session], async () => {})
    testScheduler.runNext()
    await completed

    expect(requests).toEqual([[checkpoint(session, null)]])
    expect(testScheduler.scheduledDelays).toEqual([0, 1000])
    expect(testScheduler.pendingDelays()).toEqual([1000])
    poller.stop()
  })
})
