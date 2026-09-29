import { afterEach, describe, expect, it, vi } from "vitest"
import {
  cancelChatReply,
  chatFileDownloadPath,
  decodeChatEvent,
  decodeChatEventTrees,
  fetchChatAgents,
  fetchChatFiles,
  fetchChatEvents,
  fetchChatSession,
  finishChatFileUpload,
  respondToChatApproval,
  sendChatMessage,
  startChatFileUpload,
  updateChatSession,
  uploadChatFile,
} from "./chat"

afterEach(() => vi.unstubAllGlobals())

describe("chat event response decoding", () => {
  const ref = {
    id: "sev_test",
    session: { id: "ses_test", workspace: { id: "wsp_test" } },
  }
  const message = {
    created_at: "2026-01-01T00:00:00.000Z",
    kind: "message.text",
    payload: { text: "Hello" },
    ref,
  }
  const failure = {
    created_at: "2026-01-01T00:00:01.000Z",
    kind: "tool.failure",
    payload: { name: "lisp", call_id: "call-1", output: "execution failed" },
    ref: { ...ref, id: "sev_failure" },
    parent: ref,
  }

  it("decodes nested events, including failures without a code", () => {
    const trees = [
      { event: message, children: [{ event: failure, children: [] }] },
    ]
    expect(decodeChatEventTrees(trees)).toEqual(trees)
    expect(decodeChatEvent(message)).toEqual(message)
  })

  it("rejects malformed tree structure and event payloads", () => {
    expect(() => decodeChatEventTrees({})).toThrow("invalid chat event list")
    expect(() =>
      decodeChatEventTrees([{ event: message, children: {} }]),
    ).toThrow("invalid chat event children")
    expect(() =>
      decodeChatEventTrees([
        {
          event: message,
          children: [
            { event: { ...failure, payload: { name: "lisp" } }, children: [] },
          ],
        },
      ]),
    ).toThrow()
    expect(() =>
      decodeChatEvent({ ...message, kind: "agent.success", payload: {} }),
    ).toThrow()
  })
})

describe("chat transport", () => {
  it("uses encoded session paths and same-origin credentials", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const file = new File(["contents"], "report.txt", { type: "text/plain" })
    await fetchChatSession("wsp/test", "ses/test")
    await updateChatSession("wsp/test", "ses/test", { name: "Renamed chat" })
    await fetchChatEvents("wsp/test", "ses/test")
    await fetchChatFiles("wsp/test", "ses/test")
    await fetchChatAgents("wsp/test")
    await sendChatMessage("wsp/test", "ses/test", {
      text: "Hello",
      agents: ["agt/test"],
      attachments: ["fil/test"],
    })
    await cancelChatReply("wsp/test", "ses/test", "msg/test")
    await respondToChatApproval("wsp/test", "ses/test", "apr/test", "approved")
    await startChatFileUpload("wsp/test", "ses/test", file)
    await uploadChatFile("https://uploads.example.test/file", file)
    await finishChatFileUpload("wsp/test", "ses/test", "fil/test")

    expect(fetch).toHaveBeenNthCalledWith(
      1,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest",
      { credentials: "same-origin" },
    )
    expect(fetch).toHaveBeenNthCalledWith(
      2,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest",
      expect.objectContaining({ method: "PATCH", credentials: "same-origin" }),
    )
    expect(fetch).toHaveBeenNthCalledWith(
      3,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/events?view=transcript&limit=100",
      { credentials: "same-origin" },
    )
    expect(fetch).toHaveBeenNthCalledWith(
      4,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/files",
      { credentials: "same-origin" },
    )
    expect(fetch).toHaveBeenNthCalledWith(
      5,
      "/api/v1/workspaces/wsp%2Ftest/agents",
      { credentials: "same-origin" },
    )
    expect(fetch).toHaveBeenNthCalledWith(
      6,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/messages",
      expect.objectContaining({ method: "POST", credentials: "same-origin" }),
    )
    expect(fetch).toHaveBeenNthCalledWith(
      7,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/messages/msg%2Ftest/cancel",
      { method: "POST", credentials: "same-origin" },
    )
    expect(fetch).toHaveBeenNthCalledWith(
      8,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/approvals/apr%2Ftest",
      expect.objectContaining({ method: "POST", credentials: "same-origin" }),
    )
    expect(fetch).toHaveBeenNthCalledWith(
      9,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/files",
      expect.objectContaining({ method: "POST", credentials: "same-origin" }),
    )
    expect(fetch).toHaveBeenNthCalledWith(
      10,
      "https://uploads.example.test/file",
      expect.objectContaining({ method: "PUT", body: file }),
    )
    expect(fetch).toHaveBeenNthCalledWith(
      11,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/files/fil%2Ftest/finish",
      { method: "POST", credentials: "same-origin" },
    )
    expect(chatFileDownloadPath("wsp/test", "ses/test", "fil/test")).toBe(
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/files/fil%2Ftest/download",
    )
  })

  it("forwards a route abort signal", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const controller = new AbortController()

    await fetchChatSession("wsp_a", "ses_a", controller.signal)
    await fetchChatEvents("wsp_a", "ses_a", controller.signal)

    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/workspaces/wsp_a/sessions/ses_a",
      { credentials: "same-origin", signal: controller.signal },
    )
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/workspaces/wsp_a/sessions/ses_a/events?view=transcript&limit=100",
      { credentials: "same-origin", signal: controller.signal },
    )
  })
})
