import { afterEach, describe, expect, it, vi } from "vitest"
import { importClientModule } from "../../testing/client-module"

afterEach(() => vi.unstubAllGlobals())
const message = (id: string) => ({
  created_at: "2026-01-01T00:00:00.000Z",
  kind: "message.text",
  payload: { text: id },
  ref: { id, session: { id: "session", workspace: { id: "workspace" } } },
})

describe("conversation request ownership", () => {
  it("retains an empty success, composer draft and current events across failed and out-of-order refreshes", async () => {
    const { createChatController } = await importClientModule(
      new URL("./chat-controller.svelte.ts", import.meta.url),
    )
    const request = vi.fn(async () => Response.json([]))
    vi.stubGlobal("fetch", request)
    const controller = createChatController({
      isNearBottom: () => false,
      followLatest: vi.fn(async () => {}),
      focusComposer: vi.fn(),
    })
    controller.start("workspace", "session", new AbortController().signal)
    await vi.waitFor(() => expect(controller.state.status).toBe("ready"))
    controller.state.messageText = "Unsaved message"
    let decode!: (value: object[]) => void
    request.mockImplementationOnce(async () => ({
      status: 200,
      ok: true,
      json: () =>
        new Promise((resolve) => {
          decode = resolve
        }),
    }))
    const old = controller.refreshEvents()
    await vi.waitFor(() => expect(decode).toBeTypeOf("function"))
    request.mockResolvedValueOnce(
      Response.json([{ event: message("current"), children: [] }]),
    )
    expect(await controller.refreshEvents()).toBe(true)
    decode([{ event: message("obsolete"), children: [] }])
    expect(await old).toBe(false)
    expect(controller.state.events[0].event.ref.id).toBe("current")
    let finish!: (response: Response) => void
    request.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const pending = controller.refreshEvents()
    expect(controller.state.status).toBe("ready")
    expect(controller.state.reloading).toBe(true)
    finish(new Response(null, { status: 503 }))
    expect(await pending).toBe(false)
    expect(controller.state.events[0].event.ref.id).toBe("current")
    expect(controller.state.messageText).toBe("Unsaved message")
    expect(controller.state.reloadError).not.toBe("")
    controller.stop()
  }, 15000)

  it("does not clear or focus a new chat's composer when an old send finishes decoding", async () => {
    const { createChatController } = await importClientModule(
      new URL("./chat-controller.svelte.ts", import.meta.url),
    )
    const request = vi.fn(async () => Response.json([]))
    vi.stubGlobal("fetch", request)
    const focus = vi.fn()
    const controller = createChatController({
      isNearBottom: () => true,
      followLatest: vi.fn(async () => {}),
      focusComposer: focus,
    })
    controller.start("workspace", "first", new AbortController().signal)
    await vi.waitFor(() => expect(controller.state.status).toBe("ready"))
    let decode!: (value: object) => void
    request.mockImplementationOnce(async () => ({
      status: 200,
      ok: true,
      json: () =>
        new Promise((resolve) => {
          decode = resolve
        }),
    }))
    controller.state.messageText = "Old message"
    const sending = controller.sendMessage()
    await vi.waitFor(() => expect(decode).toBeTypeOf("function"))
    controller.start("workspace", "second", new AbortController().signal)
    await vi.waitFor(() => expect(controller.state.status).toBe("ready"))
    controller.state.messageText = "New unsaved message"
    decode(message("old-send"))
    await sending
    expect(controller.state.messageText).toBe("New unsaved message")
    expect(controller.state.events).toEqual([])
    expect(controller.state.authenticationRequired).toBe(false)
    expect(focus).not.toHaveBeenCalled()
    controller.stop()
  })
})
