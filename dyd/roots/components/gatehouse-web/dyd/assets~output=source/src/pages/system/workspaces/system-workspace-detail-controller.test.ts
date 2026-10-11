import { afterEach, describe, expect, it, vi } from "vitest"
import { clearAccessToken } from "../../../app/api"
import { createSystemWorkspaceDetailController } from "./system-workspace-detail-controller.svelte"

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

describe("system workspace detail controller", () => {
  it("ignores a save response after navigation to another workspace", async () => {
    const first = {
      id: "wsp_01abc",
      alias: "first",
      name: "First",
      enabled: true,
    }
    const second = {
      id: "wsp_02abc",
      alias: "second",
      name: "Second",
      enabled: true,
    }
    let finishUpdate: (response: Response) => void = () => {}
    const pendingUpdate = new Promise<Response>((resolve) => {
      finishUpdate = resolve
    })
    let updateStarted = false
    vi.stubGlobal(
      "fetch",
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === "PATCH") {
          updateStarted = true
          return pendingUpdate
        }
        return Promise.resolve(
          jsonResponse(String(input).endsWith(first.id) ? first : second),
        )
      }),
    )

    const controller = createSystemWorkspaceDetailController({
      onAuthenticationLost: vi.fn(),
      onSystemAccessChange: vi.fn(),
    })
    const stopFirstLoad = controller.load(first.id)
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(controller.state.workspace?.id).toBe(first.id)

    controller.state.form.name = "Edited first"
    const saving = controller.save(first.id)
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(updateStarted).toBe(true)

    stopFirstLoad()
    const stopSecondLoad = controller.load(second.id)
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(controller.state.workspace?.id).toBe(second.id)

    finishUpdate(jsonResponse({ ...first, name: "Edited first" }))
    await saving
    expect(controller.state.workspace?.id).toBe(second.id)
    expect(controller.state.workspace?.name).toBe("Second")
    expect(controller.state.saving).toBe(false)
    stopSecondLoad()
  })
})
