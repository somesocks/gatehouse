import { afterEach, describe, expect, it, vi } from "vitest"
import { importClientModule } from "../../testing/client-module"
import { recipientStorageKey } from "./message-targets"

afterEach(() => vi.unstubAllGlobals())

function environment() {
  const saved = new Map<string, string>()
  vi.stubGlobal("window", { localStorage: {
    getItem: (key: string) => saved.get(key) ?? null,
    setItem: (key: string, value: string) => saved.set(key, value),
  } })
  const options = [
    { ref: { kind: "agent", id: "wag_one" }, alias: "assistant" },
    { ref: { kind: "principal", id: "prn_alex" }, name: "Alex" },
    { ref: { kind: "group", id: "grp_team" }, alias: "team" },
  ]
  const request = vi.fn(async (path: string) => {
    const url = new URL(path, "https://gatehouse.test")
    if (url.pathname.endsWith("/agents")) return Response.json([{ id: "wag_one", alias: "assistant", default: true }])
    return Response.json({ targets: options.filter((target) => target.ref.kind === url.searchParams.get("kind")) })
  })
  vi.stubGlobal("fetch", request)
  return { saved, options, request }
}

describe("sticky conversation recipients", () => {
  it("restores personal selections, preserves clears over legacy defaults, and marks missing targets unavailable", async () => {
    const { saved, options } = environment()
    const { createRecipientController } = await importClientModule(new URL("./recipient-controller.svelte.ts", import.meta.url))
    const controller = createRecipientController()
    const signal = new AbortController().signal
    controller.start("alice", "workspace", "chat", signal, null)
    await vi.waitFor(() => expect(controller.state.ready).toBe(true))
    expect(controller.state.targets).toEqual([options[0].ref])
    controller.select(options.map((target) => target.ref))
    controller.start("alice", "workspace", "chat", signal)
    await vi.waitFor(() => expect(controller.canSend).toBe(true))
    expect(controller.state.targets).toEqual(options.map((target) => target.ref))
    controller.select([])
    controller.start("alice", "workspace", "chat", signal, "wag_one")
    await vi.waitFor(() => expect(controller.state.ready).toBe(true))
    expect(controller.state.targets).toEqual([])
    controller.start("bob", "workspace", "chat", signal)
    await vi.waitFor(() => expect(controller.state.ready).toBe(true))
    expect(controller.state.targets).toEqual([])
    saved.set(recipientStorageKey("alice", "workspace", "other"), JSON.stringify({ version: 1, targets: [{ kind: "agent", id: "wag_removed" }] }))
    controller.start("alice", "workspace", "other", signal)
    await vi.waitFor(() => expect(controller.state.ready).toBe(true))
    expect(controller.available(controller.state.targets[0])).toBe(false)
    expect(controller.canSend).toBe(false)
    controller.stop()
  })

  it("does not let an old conversation's directory overwrite a new conversation", async () => {
    const { request, options } = environment()
    let finish!: (response: Response) => void
    request.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    const { createRecipientController } = await importClientModule(new URL("./recipient-controller.svelte.ts", import.meta.url))
    const controller = createRecipientController()
    const signal = new AbortController().signal
    controller.start("alice", "workspace", "old", signal)
    controller.start("alice", "workspace", "new", signal)
    await vi.waitFor(() => expect(controller.state.ready).toBe(true))
    controller.select([options[1].ref])
    finish(Response.json({ targets: [{ ref: { kind: "agent", id: "wag_obsolete" }, alias: "obsolete" }] }))
    await new Promise((resolve) => setTimeout(resolve, 10))
    expect(controller.state.options).toEqual(expect.arrayContaining(options))
    expect(controller.state.options.some((target: { alias?: string }) => target.alias === "obsolete")).toBe(false)
    expect(controller.state.targets).toEqual([options[1].ref])
    controller.stop()
  })

  it("retains the selected recipients after a failed directory refresh", async () => {
    const { request, options } = environment()
    const { createRecipientController } = await importClientModule(new URL("./recipient-controller.svelte.ts", import.meta.url))
    const controller = createRecipientController()
    controller.start("alice", "workspace", "chat", new AbortController().signal)
    await vi.waitFor(() => expect(controller.state.ready).toBe(true))
    controller.select([options[0].ref])
    request.mockRejectedValueOnce(new Error("offline"))
    expect(await controller.refresh()).toBe(false)
    expect(controller.state.targets).toEqual([options[0].ref])
    expect(controller.state.error).not.toBe("")
    expect(await controller.refresh()).toBe(true)
    expect(controller.canSend).toBe(true)
    controller.stop()
  })
})
