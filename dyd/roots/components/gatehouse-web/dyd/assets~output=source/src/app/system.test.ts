import { afterEach, describe, expect, it, vi } from "vitest"
import { clearAccessToken } from "./api"
import { createSystemWorkspace, fetchSystemWorkspaces } from "./system"

afterEach(() => {
  clearAccessToken()
  vi.unstubAllGlobals()
})

describe("system workspace API", () => {
  it("lists and creates workspaces through the system administration endpoint", async () => {
    const request = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(null, { status: 200 }),
    )
    vi.stubGlobal("fetch", request)

    await fetchSystemWorkspaces("research", "wsp_cursor")
    await createSystemWorkspace({
      alias: "research",
      name: "Research",
      enabled: true,
    })

    expect(request.mock.calls[0][0]).toBe(
      "/api/v1/system/workspaces?limit=50&name=research&cursor=wsp_cursor",
    )
    expect(request.mock.calls[1][0]).toBe("/api/v1/system/workspaces")
    expect(request.mock.calls[1][1]).toMatchObject({
      method: "POST",
      body: JSON.stringify({
        alias: "research",
        name: "Research",
        enabled: true,
      }),
    })
  })
})
