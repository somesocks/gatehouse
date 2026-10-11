import { afterEach, describe, expect, it, vi } from "vitest"
import { clearAccessToken } from "../../../app/api"
import { createWorkspaceStorageBindingsController } from "./workspace-storage-bindings-controller.svelte"

afterEach(() => {
  clearAccessToken()
  vi.unstubAllGlobals()
})

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  })
}

describe("workspace storage binding list controller", () => {
  it("loads one workspace page at a time", async () => {
    const workspaceID = "wsp_01abc"
    const first = {
      workspace: workspaceID,
      provider: "stp_01abc",
      provider_alias: "documents",
      revision: 1,
      priority: 1,
      enabled: true,
    }
    const second = {
      ...first,
      provider: "stp_02abc",
      provider_alias: "archive",
    }
    const request = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.includes("cursor=stp_cursor"))
        return jsonResponse({ bindings: [second] })
      return jsonResponse({ bindings: [first], next_cursor: "stp_cursor" })
    })
    vi.stubGlobal("fetch", request)
    const activity = { subscribe: vi.fn(() => () => {}) }
    const controller = createWorkspaceStorageBindingsController({
      activity: activity as never,
      workspaceID,
      onAuthenticationLost: vi.fn(),
      onSystemAccessChange: vi.fn(),
    })

    await controller.load()
    expect(controller.state.bindings).toEqual([first])
    expect(String(request.mock.calls[0][0])).toContain(
      `workspace=${workspaceID}&limit=50`,
    )
    await controller.loadMore()
    expect(
      controller.state.bindings.map((binding) => binding.provider),
    ).toEqual([first.provider, second.provider])
    expect(request).toHaveBeenCalledTimes(2)
  })
})
