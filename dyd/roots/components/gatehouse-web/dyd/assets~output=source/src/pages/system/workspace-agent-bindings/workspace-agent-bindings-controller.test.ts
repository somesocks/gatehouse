import { afterEach, describe, expect, it, vi } from "vitest"
import { clearAccessToken } from "../../../app/api"
import { createWorkspaceAgentBindingsController } from "./workspace-agent-bindings-controller.svelte"

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

describe("workspace agent binding list controller", () => {
  it("loads one workspace page at a time", async () => {
    const workspaceID = "wsp_01abc"
    const first = {
      id: "wag_01abc",
      workspace: workspaceID,
      alias: "assistant",
      model: "amd_01abc",
      revision: 1,
      default: true,
      enabled: true,
    }
    const second = { ...first, id: "wag_02abc", alias: "reviewer" }
    const request = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.includes("cursor=wag_cursor"))
        return jsonResponse({ agents: [second] })
      return jsonResponse({ agents: [first], next_cursor: "wag_cursor" })
    })
    vi.stubGlobal("fetch", request)
    const activity = { subscribe: vi.fn(() => () => {}) }
    const controller = createWorkspaceAgentBindingsController({
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
    expect(controller.state.bindings.map((binding) => binding.id)).toEqual([
      first.id,
      second.id,
    ])
    expect(request).toHaveBeenCalledTimes(2)
  })
})
