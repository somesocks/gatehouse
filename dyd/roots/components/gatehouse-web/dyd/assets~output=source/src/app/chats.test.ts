import { afterEach, describe, expect, it, vi } from "vitest"
import { createChat, createProjectChat, fetchChats } from "./chats"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("chats transport", () => {
  it("searches chats with session credentials and route cancellation", async () => {
    const fetch = vi.fn(async () => Response.json({ sessions: [] }))
    vi.stubGlobal("fetch", fetch)
    const controller = new AbortController()

    await fetchChats("wsp/test", "project chat", "ses_1", controller.signal)

    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/workspaces/wsp%2Ftest/sessions?limit=50&name=project+chat&cursor=ses_1",
      { credentials: "same-origin", signal: controller.signal },
    )
  })

  it("creates workspace chats", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const controller = new AbortController()

    await createChat("wsp/test", controller.signal)

    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/workspaces/wsp%2Ftest/sessions",
      {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({}),
        signal: controller.signal,
      },
    )
  })

  it("creates chats scoped to a project", async () => {
    const fetch = vi.fn(async () => Response.json({ sessions: [] }))
    vi.stubGlobal("fetch", fetch)

    await createProjectChat("wsp/test", "prj/test")

    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/workspaces/wsp%2Ftest/sessions",
      {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ project: "prj/test" }),
        signal: undefined,
      },
    )
  })
})
