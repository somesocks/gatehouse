import { describe, expect, it } from "vitest"
import type { ChatEvent, ChatEventTree } from "../../app/chat"
import {
  agentRequests,
  deliveredAgentIDs,
  finalReplies,
  hasCancellationFailure,
  inputRequests,
  inputResponse,
  displayedActivityEvents,
  pendingReplyInputRequests,
  replyCanBeCancelled,
  thinkingRateLimitDelayUntil,
} from "./chat-controller.svelte"

function tree<K extends ChatEvent["kind"]>(
  kind: K,
  children: ChatEventTree[] = [],
  payload: Record<string, unknown> = {},
): ChatEventTree<K> {
  const defaults: Record<string, Record<string, unknown>> = {
    "agent.request": { agent: "wag_test" },
    "agent.success": { text: "Reply" },
    "tool.request": { name: "lisp", reason: "Test task" },
    "input.request": { description: "Review" },
    "thinking.update": { reason: "rate_limit" },
  }
  return {
    event: {
      created_at: "2026-01-01T00:00:00.000Z",
      kind,
      payload: { ...defaults[kind], ...payload },
      ref: {
        id: kind,
        session: { id: "ses_test", workspace: { id: "wsp_test" } },
      },
      ...(kind === "agent.success"
        ? { author_agent: { id: "wag_test", workspace: { id: "wsp_test" } } }
        : {}),
    } as ChatEventTree<K>["event"],
    children,
  }
}

describe("chat request activity", () => {
  it("keeps input requests under their tool, like approvals", () => {
    const input = tree(
      "input.request",
      [tree("input.failure", [], { code: "cancelled" })],
      { description: "Review" },
    )
    const tool = tree("tool.request", [input])
    const agent = tree("agent.request", [tool])
    expect(inputRequests(agent)).toEqual([])
    expect(inputRequests(tool)).toEqual([input])
    const response = inputResponse(input)
    expect(response?.event.kind).toBe("input.failure")
    if (response?.event.kind === "input.failure")
      expect(response.event.payload.code).toBe("cancelled")
  })
  it("does not treat a human-only message as an active agent request", () => {
    const message = tree("message.text")

    expect(agentRequests(message)).toEqual([])
    expect(replyCanBeCancelled(message)).toBe(false)
  })

  it("finds form cards independently of collapsed tool activity", () => {
    const input = tree("input.request")
    const tool = tree("tool.request", [input])
    const agent = tree("agent.request", [
      tool,
      ...Array.from({ length: 6 }, () => tree("thinking.request")),
    ])
    expect(displayedActivityEvents(agent, new Set())).not.toContain(tool)
    expect(pendingReplyInputRequests(agent)).toEqual([input])
  })

  it("keeps nested pending forms in their requesting agent's branch", () => {
    const first = tree("input.request")
    const second = tree("input.request")
    const other = tree("input.request")
    const agent = tree("agent.request", [tree("tool.request", [
      first, tree("tool.request", [second]),
    ])])
    const otherAgent = tree("agent.request", [tree("tool.request", [other])])
    expect(pendingReplyInputRequests(agent)).toEqual([first, second])
    expect(pendingReplyInputRequests(otherAgent)).toEqual([other])
  })

  it.each([
    ["input.success", {}],
    ["input.failure", { code: "cancelled" }],
    ["input.failure", { code: "failed" }],
  ] as const)("keeps %s in tool activity while removing its form card", (kind, payload) => {
    const resolved = tree("input.request", [tree(kind, [], payload)])
    const pending = tree("input.request")
    const tool = tree("tool.request", [resolved, pending])
    const agent = tree("agent.request", [tool])
    expect(inputRequests(tool)).toEqual([resolved, pending])
    expect(inputResponse(resolved)?.event.kind).toBe(kind)
    expect(pendingReplyInputRequests(agent)).toEqual([pending])
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
      tree("cancel.request", [
        tree("cancel.failure", [], { code: "already_completed" }),
      ]),
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
