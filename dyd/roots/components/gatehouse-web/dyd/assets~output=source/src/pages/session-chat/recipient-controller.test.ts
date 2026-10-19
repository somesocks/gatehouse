import { afterEach, describe, expect, it, vi } from "vitest"
import { clientCode, importClientModule } from "../../testing/client-module"
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
    if (url.pathname.endsWith("/agents")) throw new Error("Recipient initialization must not look up workspace defaults")
    return Response.json({ targets: options.filter((target) => target.ref.kind === url.searchParams.get("kind")) })
  })
  vi.stubGlobal("fetch", request)
  return { saved, options, request }
}

describe("sticky conversation recipients", () => {
  it("starts without selected recipients and only looks up chat recipient options", async () => {
    const { options, request } = environment()
    const { createRecipientController } = await importClientModule(new URL("./recipient-controller.svelte.ts", import.meta.url))
    const controller = createRecipientController()
    controller.start("alice", "workspace", "chat", new AbortController().signal)
    expect(controller.state.targets).toEqual([])
    expect(controller.canSend).toBe(true)
    await vi.waitFor(() => expect(controller.state.ready).toBe(true))
    expect(controller.state.options).toEqual(expect.arrayContaining(options))
    expect(controller.state.targets).toEqual([])
    expect(request.mock.calls).toHaveLength(3)
    expect(request.mock.calls.every(([path]) => new URL(path, "https://gatehouse.test").pathname.endsWith("/recipients"))).toBe(true)
    controller.stop()
  })

  it("reactively unlocks Send when the controller starts after its first disabled render", async () => {
    const { options } = environment()
    const { createRecipientController } = await importClientModule(new URL("./recipient-controller.svelte.ts", import.meta.url))
    const code = clientCode(`
      import { flushSync } from "svelte";
      export { flushSync };
      export function observe(controller) {
        const draft = $state({ text: "", sending: false });
        const disabled = [];
        const dispose = $effect.root(() => {
          $effect(() => {
            disabled.push(draft.sending || !controller.canSend || draft.text.trim() === "");
          });
        });
        return { draft, disabled, dispose };
      }
    `)
    const { observe, flushSync } = await import(
      /* @vite-ignore */ `data:text/javascript;base64,${Buffer.from(code).toString("base64")}`
    )
    const controller = createRecipientController()
    const binding = observe(controller)
    try {
      flushSync()
      expect(binding.disabled.at(-1)).toBe(true)
      controller.start("alice", "workspace", "chat", new AbortController().signal)
      binding.draft.text = "Hello"
      flushSync()
      expect(binding.disabled.at(-1)).toBe(false)
      await vi.waitFor(() => expect(controller.state.ready).toBe(true))
      controller.select([{ kind: "principal", id: "prn_unknown" }])
      flushSync()
      expect(binding.disabled.at(-1)).toBe(true)
      controller.select([options[0].ref])
      flushSync()
      expect(binding.disabled.at(-1)).toBe(false)
      binding.draft.sending = true
      flushSync()
      expect(binding.disabled.at(-1)).toBe(true)
      binding.draft.sending = false
      binding.draft.text = ""
      flushSync()
      expect(binding.disabled.at(-1)).toBe(true)
      binding.draft.text = "Follow-up"
      flushSync()
      expect(binding.disabled.at(-1)).toBe(false)
      controller.stop()
      flushSync()
      expect(binding.disabled.at(-1)).toBe(true)
    } finally {
      binding.dispose()
      controller.stop()
    }
  })

  it("keeps a validated target sendable during background refresh and rejects it if the refreshed directory removes it", async () => {
    const { options, request } = environment()
    let delayed = false
    let removed = false
    let finish!: () => void
    const pending = new Promise<void>((resolve) => { finish = resolve })
    request.mockImplementation(async (path: string) => {
      const url = new URL(path, "https://gatehouse.test")
      const outside = url.searchParams.get("scope") === "out"
      if (outside && delayed) await pending
      const source = outside ? options.slice(1).filter((target) => !removed || target.ref.id !== options[1].ref.id) : options.slice(0, 1)
      return Response.json({ targets: source.filter((target) => target.ref.kind === url.searchParams.get("kind")), can_manage_participants: true })
    })
    const { createRecipientController } = await importClientModule(new URL("./recipient-controller.svelte.ts", import.meta.url))
    const controller = createRecipientController()
    controller.start("alice", "workspace", "chat", new AbortController().signal)
    await vi.waitFor(() => expect(controller.state.outsideReady).toBe(true))
    controller.select([options[1].ref])
    expect(controller.canSend).toBe(true)
    delayed = true
    request.mockClear()
    const refreshing = controller.refresh()
    await vi.waitFor(() => expect(request.mock.calls.some(([path]) => path.includes("scope=out"))).toBe(true))
    expect(controller.state.outsideReady).toBe(true)
    expect(controller.canSend).toBe(true)
    controller.select([{ kind: "principal", id: "prn_unknown" }])
    expect(controller.canSend).toBe(false)
    controller.select([options[1].ref])
    expect(controller.canSend).toBe(true)
    removed = true
    finish()
    expect(await refreshing).toBe(true)
    expect(controller.state.targets).toEqual([options[1].ref])
    expect(controller.available(options[1].ref)).toBe(false)
    expect(controller.canSend).toBe(false)
    controller.stop()
    expect(controller.state.outsideReady).toBe(false)
  })

  it("keeps ordinary messages sendable while outside lookup is pending or failing", async () => {
    const { options, request } = environment()
    let phase: "pending" | "failed" | "healthy" = "pending"
    let finish!: () => void
    const pending = new Promise<void>((resolve) => { finish = resolve })
    request.mockImplementation(async (path: string) => {
      const url = new URL(path, "https://gatehouse.test")
      const outside = url.searchParams.get("scope") === "out"
      if (outside) {
        if (phase === "pending") await pending
        if (phase !== "healthy") return new Response(null, { status: 503 })
      }
      const source = outside ? options.slice(1) : options.slice(0, 1)
      return Response.json({ targets: source.filter((target) => target.ref.kind === url.searchParams.get("kind")), can_manage_participants: true })
    })
    const { createRecipientController } = await importClientModule(new URL("./recipient-controller.svelte.ts", import.meta.url))
    const controller = createRecipientController()
    controller.start("alice", "workspace", "chat", new AbortController().signal)
    expect(controller.canSend).toBe(true)
    await vi.waitFor(() => expect(controller.state.ready).toBe(true))
    expect(controller.state.outsideReady).toBe(false)
    controller.select([options[0].ref])
    expect(controller.canSend).toBe(true)
    controller.select([options[1].ref])
    expect(controller.canSend).toBe(false)
    phase = "failed"
    finish()
    await vi.waitFor(() => expect(controller.state.outsideError).not.toBe(""))
    expect(controller.state.error).toBe("")
    expect(controller.state.inChat).toEqual([options[0]])
    expect(controller.canSend).toBe(false)
    controller.select([options[0].ref])
    expect(controller.canSend).toBe(true)
    controller.select([])
    expect(controller.canSend).toBe(true)
    controller.select([options[1].ref])
    phase = "healthy"
    expect(await controller.refresh()).toBe(true)
    expect(controller.state.outsideReady).toBe(true)
    expect(controller.state.outsideError).toBe("")
    expect(controller.state.targets).toEqual([options[1].ref])
    expect(controller.canSend).toBe(true)
    // A later outside outage must not trust a previously cached outside target.
    phase = "failed"
    expect(await controller.refresh()).toBe(true)
    expect(controller.state.outsideError).not.toBe("")
    expect(controller.target(options[1].ref).name).toBe("Alex")
    expect(controller.canSend).toBe(false)
    controller.select([options[0].ref])
    expect(controller.canSend).toBe(true)
    controller.stop()
  })

  it("allows clearing recipients while lookup is pending and after it fails", async () => {
    const { saved, options, request } = environment()
    saved.set(recipientStorageKey("alice", "workspace", "chat"), JSON.stringify({ version: 1, targets: [options[0].ref] }))
    let finish!: (response: Response) => void
    request.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    const { createRecipientController } = await importClientModule(new URL("./recipient-controller.svelte.ts", import.meta.url))
    const controller = createRecipientController()
    controller.start("alice", "workspace", "chat", new AbortController().signal)
    expect(controller.canSend).toBe(false)
    controller.select([])
    expect(controller.canSend).toBe(true)
    finish(new Response(null, { status: 503 }))
    await vi.waitFor(() => expect(controller.state.error).not.toBe(""))
    expect(controller.canSend).toBe(true)
    controller.select([options[0].ref])
    expect(controller.canSend).toBe(false)
    controller.stop()
  })

  it("ignores an outside-directory failure from a previously opened chat", async () => {
    const { options, request } = environment()
    let finish!: () => void
    const pending = new Promise<void>((resolve) => { finish = resolve })
    request.mockImplementation(async (path: string) => {
      const url = new URL(path, "https://gatehouse.test")
      if (url.pathname.includes("/old/") && url.searchParams.get("scope") === "out") {
        await pending
        return new Response(null, { status: 503 })
      }
      return Response.json({ targets: options.filter((target) => target.ref.kind === url.searchParams.get("kind")), can_manage_participants: true })
    })
    const { createRecipientController } = await importClientModule(new URL("./recipient-controller.svelte.ts", import.meta.url))
    const controller = createRecipientController()
    const signal = new AbortController().signal
    controller.start("alice", "workspace", "old", signal)
    await vi.waitFor(() => expect(controller.state.ready).toBe(true))
    expect(controller.state.outsideReady).toBe(false)
    controller.start("alice", "workspace", "new", signal)
    await vi.waitFor(() => expect(controller.state.outsideReady).toBe(true))
    finish()
    await new Promise((resolve) => setTimeout(resolve, 10))
    expect(controller.state.outsideError).toBe("")
    expect(controller.canSend).toBe(true)
    controller.stop()
  })

  it("offers outside recipients only to managers and moves pending targets into the chat after send", async () => {
    const { options, request } = environment()
    let joined = false
    let manager = true
    request.mockImplementation(async (path: string) => {
      const url = new URL(path, "https://gatehouse.test")
      const outside = url.searchParams.get("scope") === "out"
      const matching = options.filter((target) => target.ref.kind === url.searchParams.get("kind"))
      return Response.json({ targets: outside === !joined ? matching : [], can_manage_participants: manager })
    })
    const { createRecipientController } = await importClientModule(new URL("./recipient-controller.svelte.ts", import.meta.url))
    const controller = createRecipientController()
    controller.start("alice", "workspace", "chat", new AbortController().signal)
    await vi.waitFor(() => expect(controller.state.outsideReady).toBe(true))
    controller.select([options[0].ref])
    expect(controller.state.inChat).toEqual([])
    expect(controller.state.outside).toEqual(expect.arrayContaining(options))
    expect(controller.inChat(options[0].ref)).toBe(false)
    expect(controller.canSend).toBe(true)
    joined = true
    await controller.refresh()
    expect(controller.inChat(options[0].ref)).toBe(true)
    expect(controller.state.outside).toEqual([])
    expect(controller.state.targets).toEqual([options[0].ref])
    manager = false
    joined = false
    request.mockClear()
    await controller.refresh()
    expect(controller.state.canManage).toBe(false)
    expect(controller.state.outside).toEqual([])
    expect(request.mock.calls.every(([path]) => !path.includes("scope=out"))).toBe(true)
    expect(controller.canSend).toBe(false)
    controller.stop()
  })
  it("restores personal selections, preserves explicit clears, and marks missing targets unavailable", async () => {
    const { saved, options } = environment()
    const { createRecipientController } = await importClientModule(new URL("./recipient-controller.svelte.ts", import.meta.url))
    const controller = createRecipientController()
    const signal = new AbortController().signal
    controller.start("alice", "workspace", "chat", signal)
    await vi.waitFor(() => expect(controller.state.ready).toBe(true))
    expect(controller.state.targets).toEqual([])
    controller.select(options.map((target) => target.ref))
    controller.start("alice", "workspace", "chat", signal)
    await vi.waitFor(() => expect(controller.canSend).toBe(true))
    expect(controller.state.targets).toEqual(options.map((target) => target.ref))
    controller.select([])
    controller.start("alice", "workspace", "chat", signal)
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
