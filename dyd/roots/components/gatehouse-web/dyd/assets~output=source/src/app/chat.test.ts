import { afterEach, describe, expect, it, vi } from "vitest"
import {
  cancelChatReply,
  chatFileDownloadPath,
  decodeChatEvent,
  decodeChatEventTrees,
  fetchChatRecipients,
  fetchChatFiles,
  fetchChatEvents,
  fetchChatBranch,
  decodeChatBranch,
  fetchChatSession,
  fetchChatInputLaunch,
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
    payload: { output: "execution failed" },
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
            { event: { ...failure, payload: {} }, children: [] },
          ],
        },
      ]),
    ).toThrow()
    expect(() =>
      decodeChatEvent({ ...message, kind: "agent.success", payload: {} }),
    ).toThrow()
  })

  it("decodes file-backed outcomes and requires one failure body representation", () => {
    const file = {
      id: "sfi_result", name: "result.txt", size: 8192, fingerprint: "sha256:test",
      media_type: "text/plain", author_agent: { id: "wag_test", workspace: { id: "wsp_test" } },
    }
    for (const event of [
      { ...message, kind: "tool.success", payload: { output_file: file } },
      { ...failure, payload: { code: "execution_failed", output_file: file } },
      { ...failure, kind: "thinking.failure", payload: { code: "failed", output_file: file } },
    ]) expect(decodeChatEvent(event)).toEqual(event)
    expect(() => decodeChatEvent({ ...failure, payload: { output: "duplicated", output_file: file } })).toThrow()
    expect(() => decodeChatEvent({ ...failure, payload: { output_file: { id: "sfi_incomplete" } } })).toThrow()
  })
})

describe("chat transport", () => {
  it("sends an optional reply parent without changing recipients or attachments", async () => {
    const request = vi.fn(async (_path: string, _init?: RequestInit) =>
      Response.json({}),
    )
    vi.stubGlobal("fetch", request)
    const signal = new AbortController().signal
    const input = {
      parent: "sev_answer",
      text: "Follow up",
      targets: [{ kind: "principal", id: "alex" }] as const,
      attachments: ["own-file"],
    }
    await sendChatMessage(
      "workspace",
      "session",
      { ...input, targets: [...input.targets] },
      signal,
    )
    expect(request).toHaveBeenLastCalledWith(
      "/api/v1/workspaces/workspace/sessions/session/messages",
      {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
        signal,
      },
    )
    await sendChatMessage("workspace", "session", { text: "Root", targets: [] })
    expect(JSON.parse(String(request.mock.calls.at(-1)![1]?.body))).toEqual({
      text: "Root",
      targets: [],
    })
  })
  it("loads a bounded branch using encoded selectors and the generated public contract", async () => {
    const fetch = vi.fn(async () => Response.json({ entries: [], agent_requests: [], interactions: [] }))
    vi.stubGlobal("fetch", fetch)
    const signal = new AbortController().signal
    const response = await fetchChatBranch("workspace/one", "session/one", "answer/one", "opaque +&", signal)
    expect(fetch).toHaveBeenCalledWith("/api/v1/workspaces/workspace%2Fone/sessions/session%2Fone/events/branch?limit=20&parent=answer%2Fone&cursor=opaque+%2B%26", { credentials: "same-origin", signal })
    expect(decodeChatBranch(await response.json())).toEqual({ entries: [], agent_requests: [], interactions: [] })
    expect(() => decodeChatBranch({ entries: [], agent_requests: [], interactions: null })).toThrow()
    await fetchChatBranch("workspace", "session", "", "", signal)
    expect(fetch).toHaveBeenLastCalledWith("/api/v1/workspaces/workspace/sessions/session/events/branch?limit=20", { credentials: "same-origin", signal })
  })
  it("loads all recipient pages with encoded paths and rejects repeated cursors", async () => {
    const request = vi.fn()
      .mockResolvedValueOnce(Response.json({ targets: [{ ref: { kind: "principal", id: "prn_one" }, name: "One" }], next_cursor: "prn_cursor" }))
      .mockResolvedValueOnce(Response.json({ targets: [{ ref: { kind: "principal", id: "prn_two" }, name: "Two" }] }))
    vi.stubGlobal("fetch", request)
    const signal = new AbortController().signal
    expect((await fetchChatRecipients("wsp/test", "ses/test", "principal", signal)).targets).toHaveLength(2)
    expect(request).toHaveBeenNthCalledWith(2, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/recipients?kind=principal&limit=100&cursor=prn_cursor", { credentials: "same-origin", signal })
    request.mockImplementation(async () => Response.json({ targets: [], next_cursor: "same" }))
    await expect(fetchChatRecipients("workspace", "chat", "group", signal)).rejects.toThrow("Invalid recipient cursor.")
  })
  it("uses encoded session paths and same-origin credentials", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const file = new File(["contents"], "report.txt", { type: "text/plain" })
    await fetchChatSession("wsp/test", "ses/test")
    await updateChatSession("wsp/test", "ses/test", { name: "Renamed chat" })
    await fetchChatEvents("wsp/test", "ses/test")
    await fetchChatFiles("wsp/test", "ses/test")
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
    await fetchChatInputLaunch("wsp/test", "ses/test", "sev/test")

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
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/messages",
      expect.objectContaining({ method: "POST", credentials: "same-origin" }),
    )
    expect(fetch).toHaveBeenNthCalledWith(
      6,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/messages/msg%2Ftest/cancel",
      { method: "POST", credentials: "same-origin" },
    )
    expect(fetch).toHaveBeenNthCalledWith(
      7,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/approvals/apr%2Ftest",
      expect.objectContaining({ method: "POST", credentials: "same-origin" }),
    )
    expect(fetch).toHaveBeenNthCalledWith(
      8,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/files",
      expect.objectContaining({ method: "POST", credentials: "same-origin" }),
    )
    expect(fetch).toHaveBeenNthCalledWith(
      9,
      "https://uploads.example.test/file",
      expect.objectContaining({ method: "PUT", body: file }),
    )
    expect(fetch).toHaveBeenNthCalledWith(
      10,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/files/fil%2Ftest/finish",
      { method: "POST", credentials: "same-origin" },
    )
    expect(chatFileDownloadPath("wsp/test", "ses/test", "fil/test")).toBe(
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/files/fil%2Ftest/download",
    )
    expect(fetch).toHaveBeenNthCalledWith(
      11,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/inputs/sev%2Ftest/open",
      { credentials: "same-origin" },
    )
  })

  it("forwards a route abort signal", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const controller = new AbortController()

    await fetchChatSession("wsp_a", "ses_a", controller.signal)
    await fetchChatEvents("wsp_a", "ses_a", controller.signal)
    await fetchChatInputLaunch("wsp_a", "ses_a", "sev_input", controller.signal)

    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/workspaces/wsp_a/sessions/ses_a",
      { credentials: "same-origin", signal: controller.signal },
    )
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/workspaces/wsp_a/sessions/ses_a/inputs/sev_input/open",
      { credentials: "same-origin", signal: controller.signal },
    )
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/workspaces/wsp_a/sessions/ses_a/events?view=transcript&limit=100",
      { credentials: "same-origin", signal: controller.signal },
    )
  })
})
