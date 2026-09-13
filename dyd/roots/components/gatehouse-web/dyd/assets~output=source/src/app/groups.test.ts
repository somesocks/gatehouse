import { afterEach, describe, expect, it, vi } from "vitest"
import { fetchWorkspaceGroups } from "./groups"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("groups transport", () => {
  it("loads groups with session credentials and route cancellation", async () => {
    const fetch = vi.fn(async () => Response.json([]))
    vi.stubGlobal("fetch", fetch)
    const controller = new AbortController()

    await fetchWorkspaceGroups("wsp/test", controller.signal)

    expect(fetch).toHaveBeenCalledWith("/api/v1/workspaces/wsp%2Ftest/groups", {
      credentials: "same-origin",
      signal: controller.signal,
    })
  })
})
