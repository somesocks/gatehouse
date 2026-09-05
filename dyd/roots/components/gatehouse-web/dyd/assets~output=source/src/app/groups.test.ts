import { afterEach, describe, expect, it, vi } from "vitest"
import { fetchWorkspaceGroups } from "./groups"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("groups transport", () => {
  it("loads groups with session credentials", async () => {
    const fetch = vi.fn(async () => Response.json([]))
    vi.stubGlobal("fetch", fetch)

    await fetchWorkspaceGroups("wsp/test")

    expect(fetch).toHaveBeenCalledWith("/api/v1/workspaces/wsp%2Ftest/groups", { credentials: "same-origin" })
  })
})
