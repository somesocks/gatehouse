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
  it("loads author attribution entirely from the transcript without a bot directory request", async () => {
    const { createChatController } = await importClientModule(new URL("./chat-controller.svelte.ts", import.meta.url))
    const agent = { ref: { id: "wag_archived", workspace: { id: "workspace" } }, enabled: false, alias: "archived", label: "Archived assistant" }
    const request = vi.fn(async (_path: string) => Response.json([{ event: { ...message("reply"), kind: "agent.success", payload: { text: "Historical reply" }, author_agent: agent }, children: [] }]))
    vi.stubGlobal("fetch", request)
    const controller = createChatController({ isNearBottom: () => true, followLatest: vi.fn(async () => {}), focusComposer: vi.fn() })
    controller.start("workspace", "session", new AbortController().signal)
    await vi.waitFor(() => expect(controller.state.status).toBe("ready"))
    expect(controller.state.events[0].event.author_agent).toEqual(agent)
    expect(request).toHaveBeenCalledTimes(1)
    expect(request.mock.calls[0][0]).toContain("/sessions/session/events?view=transcript")
    controller.stop()
  })

  it("preserves a new draft typed during submission and prevents a duplicate send", async () => {
    const { createChatController } = await importClientModule(new URL("./chat-controller.svelte.ts", import.meta.url))
    let finish!: (response: Response) => void
    const submitted: unknown[] = []
    const request = vi.fn(async (path: string, init?: RequestInit) => {
      if (!path.endsWith("/messages")) return Response.json([])
      submitted.push(JSON.parse(String(init?.body)))
      return new Promise<Response>((resolve) => { finish = resolve })
    })
    vi.stubGlobal("fetch", request)
    const controller = createChatController({ isNearBottom: () => true, followLatest: vi.fn(async () => {}), focusComposer: vi.fn() })
    controller.start("workspace", "session", new AbortController().signal)
    await vi.waitFor(() => expect(controller.state.status).toBe("ready"))
    controller.state.messageText = "First message"
    const sending = controller.sendMessage()
    await vi.waitFor(() => expect(finish).toBeTypeOf("function"))
    controller.state.messageText = "Next message\nStill editing"
    await controller.sendMessage()
    expect(submitted).toEqual([{ text: "First message", targets: [] }])
    expect(controller.state.sendingMessage).toBe(true)
    finish(Response.json({ ...message("sent"), payload: { text: "First message" } }))
    await sending
    expect(controller.state.messageText).toBe("Next message\nStill editing")
    expect(controller.state.sendingMessage).toBe(false)
    controller.stop()
  })

  it("sends a stable mixed target set on successive messages without interpreting text as delivery", async () => {
    const { createChatController } = await importClientModule(new URL("./chat-controller.svelte.ts", import.meta.url))
    const sent: unknown[] = []
    const request = vi.fn(async (path: string, init?: RequestInit) => {
      if (path.endsWith("/messages")) {
        const body = JSON.parse(String(init?.body))
        sent.push(body)
        return Response.json({ ...message(`sent-${sent.length}`), payload: { text: body.text, targets: body.targets.map((ref: object) => ({ ref })) } })
      }
      return Response.json([])
    })
    vi.stubGlobal("fetch", request)
    const controller = createChatController({ isNearBottom: () => true, followLatest: vi.fn(async () => {}), focusComposer: vi.fn() })
    controller.start("workspace", "session", new AbortController().signal)
    await vi.waitFor(() => expect(controller.state.status).toBe("ready"))
    const targets = [{ kind: "agent", id: "wag_one" }, { kind: "principal", id: "prn_alex" }, { kind: "group", id: "grp_team" }]
    controller.state.messageText = "First"
    await controller.sendMessage(targets)
    expect(controller.state.messageText).toBe("")
    controller.state.messageText = "Follow up"
    await controller.sendMessage(targets)
    expect(sent).toEqual([{ text: "First", targets }, { text: "Follow up", targets }])
    expect(targets).toHaveLength(3)
    controller.state.messageText = "@assistant is just text after clearing recipients"
    await controller.sendMessage([])
    expect(sent[2]).toEqual({ text: "@assistant is just text after clearing recipients", targets: [] })
    controller.state.messageText = "Keep this draft"
    request.mockResolvedValueOnce(new Response("Recipient unavailable", { status: 400 }))
    await controller.sendMessage(targets)
    expect(controller.state.messageText).toBe("Keep this draft")
    expect(controller.state.messageError).toBe("Recipient unavailable")
    controller.stop()
  })
  it("retains an empty success, composer draft and current events across failed and out-of-order refreshes", async () => {
    const { createChatController } = await importClientModule(
      new URL("./chat-controller.svelte.ts", import.meta.url),
    )
    const request = vi.fn(async () => Response.json([]))
    vi.stubGlobal("fetch", request)
    const followLatest = vi.fn(async () => {})
    const controller = createChatController({
      isNearBottom: () => false,
      followLatest,
      focusComposer: vi.fn(),
    })
    controller.start("workspace", "session", new AbortController().signal)
    await vi.waitFor(() => expect(controller.state.status).toBe("ready"))
    expect(followLatest).toHaveBeenCalledWith("instant")
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
