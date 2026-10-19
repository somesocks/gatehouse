import { describe, expect, it } from "vitest"
import { decodeChatEvent } from "../../app/chat"
import { agentLabel } from "./agent-attribution"

describe("transcript bot attribution", () => {
  it("renders a removed or disabled author's hydrated name without a participant directory", () => {
    const agent = {
      ref: { id: "wag_archived", workspace: { id: "wsp_chat" } },
      alias: "archived",
      label: "Archived assistant",
      enabled: false,
    }
    const event = decodeChatEvent({
      kind: "agent.success",
      created_at: "2026-01-01T00:00:00Z",
      ref: {
        id: "sev_reply",
        session: { id: "ses_chat", workspace: { id: "wsp_chat" } },
      },
      payload: { text: "Historical reply" },
      author_agent: agent,
    })
    expect(agentLabel(event.author_agent)).toBe("Archived assistant")
    expect(agentLabel({ ...agent, label: undefined })).toBe("archived")
    expect(agentLabel({ ref: agent.ref, enabled: false })).toBe("wag_archived")
  })

  it("decodes queued request target metadata independently of its human author", () => {
    const agent = {
      ref: { id: "wag_target", workspace: { id: "wsp_chat" } },
      alias: "target",
      label: "Queued assistant",
      enabled: true,
    }
    const event = decodeChatEvent({
      kind: "agent.request",
      created_at: "2026-01-01T00:00:00Z",
      ref: {
        id: "sev_request",
        session: { id: "ses_chat", workspace: { id: "wsp_chat" } },
      },
      payload: { agent: "wag_target" },
      author_principal: { ref: { id: "prn_author" }, enabled: true },
      target_agent: agent,
    })
    if (event.kind !== "agent.request") throw new Error("wrong event kind")
    expect(agentLabel(event.target_agent)).toBe("Queued assistant")
    expect(event.author_agent).toBeUndefined()
    expect(event.author_principal?.ref.id).toBe("prn_author")
  })
})
