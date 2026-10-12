import { afterEach, describe, expect, it, vi } from "vitest"
import { createChatCollectionController } from "./chat-collection-controller.svelte"

afterEach(() => vi.unstubAllGlobals())

describe("bounded chat directory", () => {
  it("replaces pages and retains the project filter when navigating back", async () => {
    const fetch = vi.fn(async (path: string) => {
      const cursor = new URL(path, "http://localhost").searchParams.get(
        "cursor",
      )
      return Response.json({
        sessions: [
          { id: cursor ? "second" : "first", created_at: "2026-10-11" },
        ],
        ...(cursor ? {} : { next_cursor: "next" }),
      })
    })
    vi.stubGlobal("fetch", fetch)
    const controller = createChatCollectionController({
      onAuthenticationLost: vi.fn(),
    })
    const signal = new AbortController().signal
    controller.start("workspace", "name", signal, "project")
    await vi.waitFor(() => expect(controller.state.loading).toBe(false))
    await controller.loadMore("workspace", "name", signal)
    expect(controller.state.chats.map((chat) => chat.id)).toEqual(["second"])
    await controller.previous("workspace", "name", signal)
    expect(controller.state.chats.map((chat) => chat.id)).toEqual(["first"])
    for (const [path] of fetch.mock.calls)
      expect(
        new URL(path, "http://localhost").searchParams.get("project"),
      ).toBe("project")
    controller.stop()
  })
  it("retains the current rows and cursor history when the next request fails", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(
        Response.json({
          sessions: [{ id: "first", created_at: "2026-10-11" }],
          next_cursor: "next",
        }),
      )
      .mockResolvedValueOnce(new Response(null, { status: 500 }))
    vi.stubGlobal("fetch", fetch)
    const controller = createChatCollectionController({
      onAuthenticationLost: vi.fn(),
    })
    const signal = new AbortController().signal
    controller.start("workspace", "", signal)
    await vi.waitFor(() => expect(controller.state.loading).toBe(false))
    await controller.loadMore("workspace", "", signal)
    expect(controller.state.chats.map((chat) => chat.id)).toEqual(["first"])
    expect(controller.state.history).toEqual([])
    expect(controller.state.cursor).toBe("next")
    expect(controller.state.error).not.toBe("")
    controller.stop()
  })
})
