import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import { importClientModule } from "../../testing/client-module"
import type { ActivityRefresh, ActivitySelector } from "../../app/activity"

afterEach(() => vi.unstubAllGlobals())
let client: Awaited<ReturnType<typeof importClientModule>>
beforeAll(async () => {
  client = await importClientModule(new URL("./group-secrets-controller.svelte.ts", import.meta.url))
}, 30000)

const secret = { id: "gsc_a", group: "group", description: "Deployment token", author: { id: "owner" }, revision: 3, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" }

async function setup(canManage = true) {
  let refresh!: ActivityRefresh
  const unsubscribe = vi.fn()
  const activity = {
    subscribe: vi.fn((_selectors: Iterable<ActivitySelector>, callback: ActivityRefresh) => { refresh = callback; return unsubscribe }),
    poll: vi.fn(async () => {}), dispose: vi.fn(),
  }
  const onAuthenticationLost = vi.fn(), onAccessLost = vi.fn()
  const request = vi.fn(async (_path: string, _init?: RequestInit): Promise<Response> => Response.json({ secrets: [secret], can_manage_secrets: canManage }))
  vi.stubGlobal("fetch", request)
  const controller = client.createGroupSecretsController({ activity, onAuthenticationLost, onAccessLost })
  controller.start("workspace", "group")
  await vi.waitFor(() => expect(controller.directory.state.hasLoaded).toBe(true))
  return { controller, request, activity, unsubscribe, onAuthenticationLost, onAccessLost, refresh: () => refresh({ names: new Set(["group-secrets"]), signal: new AbortController().signal }) }
}

describe("group secret management", () => {
  it("loads metadata for members but only allows managers to open mutation dialogs", async () => {
    const { controller, request } = await setup(false)
    expect(controller.directory.state.items).toEqual([secret])
    controller.begin("create")
    controller.begin("edit", secret)
    controller.begin("remove", secret)
    expect(controller.state.dialog).toBeNull()
    expect(await controller.save()).toBe(false)
    expect(request.mock.calls.some(([, init]) => init?.method)).toBe(false)
    controller.stop()
  })

  it("clears credential drafts immediately after a saved create even if metadata refresh fails", async () => {
    const { controller, request } = await setup()
    controller.begin("create")
    controller.state.description = " New credential "
    controller.state.value = "VALUE_SENTINEL"
    request.mockImplementation(async (_path, init) => init?.method === "POST" ? Response.json(secret, { status: 201 }) : new Response("unavailable", { status: 503 }))
    expect(await controller.save()).toBe(true)
    expect(request.mock.calls.find(([, init]) => init?.method === "POST")?.[1]?.body).toBe(JSON.stringify({ description: "New credential", value: "VALUE_SENTINEL" }))
    expect(controller.state.value).toBe("")
    expect(controller.state.dialog).toBeNull()
    expect(controller.state.error).toContain("Saved")
    controller.stop()
  })

  it("preserves the original revision and draft across background refresh and conflicts", async () => {
    const { controller, request } = await setup()
    controller.begin("edit", secret)
    controller.state.description = "Draft description"
    controller.state.value = "Replacement credential"
    request.mockImplementation(async (path, init) => {
      if (init?.method === "PATCH") return new Response("This secret changed elsewhere.", { status: 409 })
      const latest = { ...secret, revision: 4, description: "Changed elsewhere" }
      return path.includes("gsc_a") ? Response.json(latest) : Response.json({ secrets: [latest], can_manage_secrets: true })
    })
    await controller.directory.reload()
    expect(controller.state.snapshot.revision).toBe(3)
    expect(controller.state.description).toBe("Draft description")
    expect(await controller.save()).toBe(false)
    expect(request.mock.calls.findLast(([, init]) => init?.method === "PATCH")?.[1]?.body).toBe(JSON.stringify({ description: "Draft description", value: "Replacement credential", expected_revision: 3 }))
    expect(controller.state.value).toBe("Replacement credential")
    await controller.reloadSecret()
    expect(controller.state.snapshot.revision).toBe(4)
    expect(controller.state.description).toBe("Changed elsewhere")
    expect(controller.state.value).toBe("")
    controller.stop()
  })

  it("edits metadata without submitting an empty replacement and removes only on confirmation", async () => {
    const { controller, request } = await setup()
    controller.begin("remove", secret)
    controller.cancelDialog()
    expect(request.mock.calls.some(([, init]) => init?.method)).toBe(false)
    request.mockImplementation(async (_path, init) => init?.method ? new Response(null, { status: 204 }) : Response.json({ secrets: [], can_manage_secrets: true }))
    controller.begin("edit", secret)
    controller.state.description = "Metadata only"
    expect(await controller.save()).toBe(true)
    expect(request.mock.calls.findLast(([, init]) => init?.method === "PATCH")?.[1]?.body).toBe(JSON.stringify({ description: "Metadata only", expected_revision: 3 }))
    controller.begin("remove", secret)
    expect(await controller.save()).toBe(true)
    expect(request.mock.calls.findLast(([, init]) => init?.method === "DELETE")?.[1]?.body).toBe(JSON.stringify({ expected_revision: 3 }))
    controller.stop()
  })

  it("cancels dialog work and ignores a late permission failure after reopening", async () => {
    const { controller, request } = await setup()
    let resolve!: (response: Response) => void
    request.mockImplementation((_path, init) => init?.method === "POST" ? new Promise((done) => { resolve = done }) : Promise.resolve(Response.json({ secrets: [secret], can_manage_secrets: true })))
    controller.begin("create")
    controller.state.description = "Old draft"
    controller.state.value = "OLD_VALUE"
    const old = controller.save()
    const signal = request.mock.calls.findLast(([, init]) => init?.method === "POST")?.[1]?.signal
    controller.cancelDialog()
    expect(signal?.aborted).toBe(true)
    expect(controller.state.value).toBe("")
    controller.begin("create")
    controller.state.value = "NEW_VALUE"
    resolve(new Response("forbidden", { status: 403 }))
    await old
    expect(controller.state.value).toBe("NEW_VALUE")
    expect(controller.state.canManage).toBe(true)
    expect(controller.state.error).toBe("")
    controller.stop()
    expect(controller.state.value).toBe("")
  })

  it("clears credential drafts when manager rights are lost and preserves metadata on transient failure", async () => {
    const { controller, request, refresh } = await setup()
    controller.begin("edit", secret)
    controller.state.value = "VALUE_SENTINEL"
    request.mockImplementation(async () => new Response("unavailable", { status: 503 }))
    await expect(refresh()).rejects.toThrow()
    expect(controller.directory.state.items).toEqual([secret])
    expect(controller.state.value).toBe("VALUE_SENTINEL")
    request.mockImplementation(async () => Response.json({ secrets: [secret], can_manage_secrets: false }))
    await refresh()
    expect(controller.state.canManage).toBe(false)
    expect(controller.state.dialog).toBeNull()
    expect(controller.state.value).toBe("")
    controller.stop()
  })

  it("rejects stale metadata after an owner change and disposes subscriptions", async () => {
    const { controller, request, unsubscribe } = await setup()
    let decode!: (value: unknown) => void
    request.mockImplementation(async () => ({ ok: true, status: 200, json: () => new Promise((resolve) => { decode = resolve }) }) as Response)
    const old = controller.directory.reload()
    await vi.waitFor(() => expect(decode).toBeTypeOf("function"))
    request.mockImplementation(async () => Response.json({ secrets: [], can_manage_secrets: false }))
    controller.start("workspace", "different-group")
    decode({ secrets: [secret], can_manage_secrets: true })
    await old
    await vi.waitFor(() => expect(controller.directory.state.hasLoaded).toBe(true))
    expect(controller.directory.state.items).toEqual([])
    expect(controller.state.canManage).toBe(false)
    controller.stop()
    expect(unsubscribe).toHaveBeenCalled()
  })

  it("ignores older permission metadata after a forbidden save and clears drafts on access loss", async () => {
    const { controller, request, onAccessLost } = await setup()
    let decode!: (value: unknown) => void
    request.mockImplementation(async (_path, init) => {
      if (init?.method === "PATCH") return new Response("forbidden", { status: 403 })
      return { ok: true, status: 200, json: () => new Promise((resolve) => { decode = resolve }) } as Response
    })
    const old = controller.directory.reload()
    await vi.waitFor(() => expect(decode).toBeTypeOf("function"))
    controller.begin("edit", secret)
    controller.state.value = "VALUE_SENTINEL"
    expect(await controller.save()).toBe(false)
    decode({ secrets: [secret], can_manage_secrets: true })
    await old
    expect(controller.state.canManage).toBe(false)
    expect(controller.state.value).toBe("")
    expect(controller.state.dialog).toBeNull()
    request.mockImplementation(async () => new Response("not found", { status: 404 }))
    await controller.directory.reload()
    expect(controller.directory.state.items).toEqual([])
    expect(onAccessLost).toHaveBeenCalledOnce()
    expect(controller.state.active).toBe(false)
    controller.stop()
  })
})
