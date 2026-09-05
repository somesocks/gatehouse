import { afterEach, describe, expect, it, vi } from "vitest"
import { fetchChats } from "./chats"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("chats transport", () => {
  it("searches chats with session credentials", async () => {
    const fetch = vi.fn(async () => Response.json({ sessions: [] }))
    vi.stubGlobal("fetch", fetch)

    await fetchChats("wsp/test", "project chat", "ses_1")

    expect(fetch).toHaveBeenCalledWith("/api/v1/workspaces/wsp%2Ftest/sessions?limit=50&name=project+chat&cursor=ses_1", { credentials: "same-origin" })
  })
})
