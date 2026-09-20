import { describe, expect, it } from "vitest"
import { mentionedAgentIDs } from "./chat-mentions"

const agents = [
  { id: "wag_research", alias: "research", label: "Research", default: true },
  { id: "wag_ops", alias: "ops/triage", label: "Operations", default: false },
  { id: "wag_build", alias: "build-bot", default: false },
]

describe("mentionedAgentIDs", () => {
  it("resolves complete mentions in their written order", () => {
    expect(
      mentionedAgentIDs("@research ask @ops/triage to review", agents),
    ).toEqual(["wag_research", "wag_ops"])
  })

  it("deduplicates repeated mentions", () => {
    expect(mentionedAgentIDs("@research @research", agents)).toEqual([
      "wag_research",
    ])
  })

  it("leaves embedded, unknown, and incomplete aliases untargeted", () => {
    expect(
      mentionedAgentIDs(
        "email@research @unknown @research-extra @build_bot",
        agents,
      ),
    ).toEqual([])
  })

  it("supports hyphenated aliases", () => {
    expect(mentionedAgentIDs("@build-bot please deploy", agents)).toEqual([
      "wag_build",
    ])
  })
})
