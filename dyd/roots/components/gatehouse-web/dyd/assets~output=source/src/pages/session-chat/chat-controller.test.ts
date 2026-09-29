import { describe, expect, it } from "vitest"
import type { ChatEventTree } from "../../app/chat"
import {
  agentRequests,
  deliveredAgentIDs,
  finalReplies,
  hasCancellationFailure,
  inputRequests,
  inputResponse,
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
      payload: kind === "agent.success" ? { text: "Reply", ...payload } : payload,
      ref: { id: kind },
      ...(kind === "agent.success"
        ? { author_agent: { id: "wag_test", workspace: { id: "wsp_test" } } }
        : {}),
    },
    children,
  }
}

describe("chat request activity", () => {
  it("keeps input requests under their tool, like approvals", () => {
    const input = tree("input.request", [tree("input.failure", [], { code: "cancelled" })], { description: "Review" })
    const tool = tree("tool.request", [input])
    const agent = tree("agent.request", [tool])
    expect(inputRequests(agent)).toEqual([])
    expect(inputRequests(tool)).toEqual([input])
    expect(inputResponse(input)?.event.payload.code).toBe("cancelled")
  })
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
    const firstReply = tree("agent.success")
    const secondReply = tree("agent.success")
    const message = tree("message.text", [
      tree("agent.request", [firstReply]),
      tree("agent.request", [secondReply]),
    ])

    expect(finalReplies(agentRequests(message)[0])).toEqual([firstReply])
    expect(finalReplies(agentRequests(message)[1])).toEqual([secondReply])
  })

  it("treats cancel.failure as a terminal cancellation outcome", () => {
    const request = tree("agent.request", [
      tree("cancel.request", [tree("cancel.failure", [], { code: "already_completed" })]),
    ])
    expect(hasCancellationFailure(request)).toBe(true)
    expect(replyCanBeCancelled(request)).toBe(false)
  })
})

describe("thinking rate-limit delay", () => {
  it("returns a future rate-limit delay deadline", () => {
    const until = "2026-01-01T00:00:30.000Z"
    const thinking = tree("thinking.request", [
      tree("thinking.update", [], { reason: "rate_limit", until }),
    ])

    expect(
      thinkingRateLimitDelayUntil(
        thinking,
        Date.parse("2026-01-01T00:00:00.000Z"),
      ),
    ).toBe(until)
  })

  it("ignores elapsed and malformed delay deadlines", () => {
    const thinking = tree("thinking.request", [
      tree("thinking.update", [], {
        reason: "rate_limit",
        until: "not-a-timestamp",
      }),
      tree("thinking.update", [], {
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
