import { describe, expect, it } from "vitest"
import type { ChatEventTree } from "../../app/chat"
import {
  agentRequests,
  deliveredAgentIDs,
  finalReplies,
  replyCanBeCancelled,
  thinkingRateLimitDelayUntil,
} from "./chat-controller.svelte"

function tree(
  kind: string,
  children: ChatEventTree[] = [],
  payload: ChatEventTree["event"]["payload"] = {},
): ChatEventTree {
  return {
    event: {
      created_at: "2026-01-01T00:00:00.000Z",
      kind,
      payload: kind === "agent.reply" ? { text: "Reply", ...payload } : payload,
      ref: { id: kind },
      ...(kind === "agent.reply"
        ? { author_agent: { id: "wag_test", workspace: { id: "wsp_test" } } }
        : {}),
    },
    children,
  }
}

describe("chat request activity", () => {
  it("does not treat a human-only message as an active agent request", () => {
    const message = tree("message.text")

    expect(agentRequests(message)).toEqual([])
    expect(replyCanBeCancelled(message)).toBe(false)
  })

  it("recognizes one request as cancellable work", () => {
    const message = tree("message.text", [tree("agent.request")])

    expect(agentRequests(message)).toHaveLength(1)
    expect(replyCanBeCancelled(message)).toBe(true)
  })

  it("recognizes an individual request as cancellable work", () => {
    expect(replyCanBeCancelled(tree("agent.request"))).toBe(true)
  })

  it("keeps replies in their request branch", () => {
    const firstReply = tree("agent.reply")
    const secondReply = tree("agent.reply")
    const message = tree("message.text", [
      tree("agent.request", [firstReply]),
      tree("agent.request", [secondReply]),
    ])

    expect(finalReplies(agentRequests(message)[0])).toEqual([firstReply])
    expect(finalReplies(agentRequests(message)[1])).toEqual([secondReply])
  })
})

describe("thinking rate-limit delay", () => {
  it("returns a future rate-limit delay deadline", () => {
    const until = "2026-01-01T00:00:30.000Z"
    const thinking = tree("thinking.started", [
      tree("thinking.delay", [], { reason: "rate_limit", until }),
    ])

    expect(
      thinkingRateLimitDelayUntil(
        thinking,
        Date.parse("2026-01-01T00:00:00.000Z"),
      ),
    ).toBe(until)
  })

  it("ignores elapsed and malformed delay deadlines", () => {
    const thinking = tree("thinking.started", [
      tree("thinking.delay", [], {
        reason: "rate_limit",
        until: "not-a-timestamp",
      }),
      tree("thinking.delay", [], {
        reason: "rate_limit",
        until: "2026-01-01T00:00:00.000Z",
      }),
    ])

    expect(
      thinkingRateLimitDelayUntil(
        thinking,
        Date.parse("2026-01-01T00:00:01.000Z"),
      ),
    ).toBeUndefined()
  })
})

describe("chat delivery", () => {
  const agents = [
    { id: "wag_default", alias: "default", default: true },
    { id: "wag_other", alias: "other", default: false },
  ]

  it("uses the direct recipient when no agent is mentioned", () => {
    expect(
      deliveredAgentIDs("Hello", agents, {
        mode: "direct",
        agentID: "wag_default",
      }),
    ).toEqual(["wag_default"])
  })

  it("replaces the direct recipient with mentions", () => {
    expect(
      deliveredAgentIDs("@other Hello", agents, {
        mode: "direct",
        agentID: "wag_default",
      }),
    ).toEqual(["wag_other"])
  })

  it("keeps unmentioned group messages human-only", () => {
    expect(deliveredAgentIDs("Hello", agents, { mode: "group" })).toEqual([])
  })
})
