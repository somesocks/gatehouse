import { afterEach, describe, expect, it, vi } from "vitest"
import { fetchSystemGrants, fetchWorkspaces } from "./access"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("access transport", () => {
  it("loads the workspace catalog with session credentials", async () => {
    const fetch = vi.fn(async () => Response.json([]))
    vi.stubGlobal("fetch", fetch)

    await fetchWorkspaces()

    expect(fetch).toHaveBeenCalledWith("/api/v1/workspaces", { credentials: "same-origin" })
  })

  it("loads system grants with session credentials", async () => {
    const fetch = vi.fn(async () => Response.json([]))
    vi.stubGlobal("fetch", fetch)

    await fetchSystemGrants()

    expect(fetch).toHaveBeenCalledWith("/api/v1/system/grants", { credentials: "same-origin" })
  })
})
