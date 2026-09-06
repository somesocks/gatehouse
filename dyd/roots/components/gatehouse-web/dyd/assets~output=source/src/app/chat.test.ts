import { afterEach, describe, expect, it, vi } from "vitest"
import { cancelChatReply, chatFileDownloadPath, fetchChatAgents, fetchChatEvents, fetchChatSession, finishChatFileUpload, respondToChatApproval, sendChatMessage, startChatFileUpload, uploadChatFile } from "./chat"

afterEach(() => vi.unstubAllGlobals())

describe("chat transport", () => {
  it("uses encoded session paths and same-origin credentials", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const file = new File(["contents"], "report.txt", { type: "text/plain" })
    await fetchChatSession("wsp/test", "ses/test")
    await fetchChatEvents("wsp/test", "ses/test")
    await fetchChatAgents("wsp/test")
    await sendChatMessage("wsp/test", "ses/test", { text: "Hello", agent: "agt/test", attachments: ["fil/test"] })
    await cancelChatReply("wsp/test", "ses/test", "msg/test")
    await respondToChatApproval("wsp/test", "ses/test", "apr/test", "approved")
    await startChatFileUpload("wsp/test", "ses/test", file)
    await uploadChatFile("https://uploads.example.test/file", file)
    await finishChatFileUpload("wsp/test", "ses/test", "fil/test")

    expect(fetch).toHaveBeenNthCalledWith(1, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest", { credentials: "same-origin" })
    expect(fetch).toHaveBeenNthCalledWith(2, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/events?limit=100", { credentials: "same-origin" })
    expect(fetch).toHaveBeenNthCalledWith(3, "/api/v1/workspaces/wsp%2Ftest/agents", { credentials: "same-origin" })
    expect(fetch).toHaveBeenNthCalledWith(4, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/messages", expect.objectContaining({ method: "POST", credentials: "same-origin" }))
    expect(fetch).toHaveBeenNthCalledWith(5, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/messages/msg%2Ftest/cancel", { method: "POST", credentials: "same-origin" })
    expect(fetch).toHaveBeenNthCalledWith(6, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/approvals/apr%2Ftest", expect.objectContaining({ method: "POST", credentials: "same-origin" }))
    expect(fetch).toHaveBeenNthCalledWith(7, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/files", expect.objectContaining({ method: "POST", credentials: "same-origin" }))
    expect(fetch).toHaveBeenNthCalledWith(8, "https://uploads.example.test/file", expect.objectContaining({ method: "PUT", body: file }))
    expect(fetch).toHaveBeenNthCalledWith(9, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/files/fil%2Ftest/finish", { method: "POST", credentials: "same-origin" })
    expect(chatFileDownloadPath("wsp/test", "ses/test", "fil/test")).toBe("/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/files/fil%2Ftest/download")
  })

  it("forwards a route abort signal", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const controller = new AbortController()

    await fetchChatSession("wsp_a", "ses_a", controller.signal)
    await fetchChatEvents("wsp_a", "ses_a", controller.signal)

    expect(fetch).toHaveBeenCalledWith("/api/v1/workspaces/wsp_a/sessions/ses_a", { credentials: "same-origin", signal: controller.signal })
    expect(fetch).toHaveBeenCalledWith("/api/v1/workspaces/wsp_a/sessions/ses_a/events?limit=100", { credentials: "same-origin", signal: controller.signal })
  })
})
