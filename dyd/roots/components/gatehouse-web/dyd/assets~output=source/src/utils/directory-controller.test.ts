import { describe, expect, it, vi } from "vitest"
import { createDirectoryController } from "./directory-controller.svelte"

describe("directory pagination", () => {
  it("replaces pages and preserves the applied query while moving forward/back", async () => {
    const fetchPage = vi.fn(
      async (
        name: string,
        cursor: string,
        status: string,
        _signal: AbortSignal,
      ) => ({
        response: new Response(null, { status: 200 }),
        items: [{ id: cursor || "first", name, status }],
        next: cursor ? "" : "cursor",
      }),
    )
    const controller = createDirectoryController({
      fetchPage,
      onAuthenticationLost: vi.fn(),
      onSystemAccessChange: vi.fn(),
    })
    controller.state.search = "production"
    controller.state.status = "enabled"
    await controller.search()
    controller.state.search = "unsubmitted draft"
    await controller.next()
    expect(controller.state.items).toEqual([
      { id: "cursor", name: "production", status: "enabled" },
    ])
    expect(controller.state.history).toEqual([""])
    await controller.previous()
    expect(controller.state.items).toEqual([
      { id: "first", name: "production", status: "enabled" },
    ])
    expect(controller.state.history).toEqual([])
    controller.stop()
  })
  it("ignores stale search results even when transport ignores abort", async () => {
    let finish: (value: {
      response: Response
      items: string[]
      next?: string
    }) => void = () => {}
    const pending = new Promise<{
      response: Response
      items: string[]
      next?: string
    }>((resolve) => {
      finish = resolve
    })
    const fetchPage = vi
      .fn()
      .mockImplementationOnce(() => pending)
      .mockResolvedValue({
        response: new Response(null, { status: 200 }),
        items: ["latest"],
      })
    const controller = createDirectoryController<string>({
      fetchPage,
      onAuthenticationLost: vi.fn(),
      onSystemAccessChange: vi.fn(),
    })
    const first = controller.search()
    controller.state.search = "new"
    await controller.search()
    finish({
      response: new Response(null, { status: 200 }),
      items: ["stale"],
      next: "old",
    })
    await first
    expect(controller.state.items).toEqual(["latest"])
    expect(controller.state.next).toBe("")
    controller.stop()
  })
})
