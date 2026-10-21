import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import { importClientModule } from "../../testing/client-module"
import type { ActivityRefresh, ActivitySelector } from "../../app/activity"

afterEach(() => vi.unstubAllGlobals())
let client: Awaited<ReturnType<typeof importClientModule>>
beforeAll(async () => {
  client = await importClientModule(new URL("./project-access-controller.svelte.ts", import.meta.url))
}, 30000)

const grant = { id: "pgr_a", principal: "alice", role: "member", enabled: true, revision: 3 }
const alice = { id: "alice", name: "Alice", enabled: true, grants: [grant] }
const developers = { id: "developers", name: "Developers", enabled: true, grants: [{ ...grant, id: "pgr_g", principal: undefined, group: "developers" }] }

function setup(canManage = true) {
  let refresh!: ActivityRefresh
  const unsubscribe = vi.fn()
  const activity = {
    subscribe: vi.fn((_selectors: Iterable<ActivitySelector>, callback: ActivityRefresh) => {
      refresh = callback
      return unsubscribe
    }),
    poll: vi.fn(async () => {}),
    dispose: vi.fn(),
  }
  const onAuthenticationLost = vi.fn(), onAccessLost = vi.fn()
  const request = vi.fn(async (path: string, _init?: RequestInit): Promise<Response> => {
    if (!path.includes("/members") && !path.includes("-options")) return Response.json({ id: "project" })
    const kind = new URL(path, "http://localhost").searchParams.get("kind")
    return Response.json({ members: kind === "group" ? [developers] : [alice], can_manage_access: canManage })
  })
  vi.stubGlobal("fetch", request)
  const controller = client.createProjectAccessController({ activity, onAuthenticationLost, onAccessLost })
  controller.start("workspace", "project")
  return { controller, request, activity, onAuthenticationLost, onAccessLost, unsubscribe, refresh: () => refresh({ names: new Set(["project-access"]), signal: new AbortController().signal }) }
}

describe("project access management", () => {
  it("loads explicit principals and groups and prevents read-only mutations", async () => {
    const { controller, request } = setup(false)
    expect(request).not.toHaveBeenCalled()
    await controller.open()
    expect(controller.people.state.items).toEqual([alice])
    expect(controller.groups.state.items).toEqual([developers])
    expect(controller.state.canManage).toBe(false)
    controller.beginAdd("principal")
    controller.beginAction("remove", "principal", grant, "Alice")
    expect(controller.state.adding).toBeNull()
    expect(controller.state.action).toBeNull()
    expect(await controller.add()).toBe(false)
    expect(request.mock.calls.every(([, init]) => !init?.method)).toBe(true)
    controller.stop()
  })

  it("adds either kind with an explicit role and preserves selection across pages", async () => {
    const { controller, request } = setup()
    await controller.open()
    for (const kind of ["principal", "group"] as const) {
      controller.beginAdd(kind)
      await controller.options(kind).retry()
      const subject = kind === "principal" ? "alice" : "developers"
      controller.select(subject)
      controller.state.role = "contributor"
      controller.options(kind).state.items = []
      expect(controller.state.selectedName).toBe(kind === "principal" ? "Alice" : "Developers")
      expect(await controller.add()).toBe(true)
      expect(request.mock.calls.findLast(([, init]) => init?.method === "POST")?.[1]?.body).toBe(JSON.stringify({ [kind]: subject, role: "contributor" }))
      expect(controller.state.adding).toBeNull()
    }
    controller.stop()
  })

  it("freezes the opening revision until an explicit reload and retains a conflicting draft", async () => {
    const { controller, request } = setup()
    await controller.open()
    controller.beginAction("role", "principal", grant, "Alice")
    controller.state.action.role = "manager"
    request.mockImplementation(async (path, init) => {
      if (init?.method === "PATCH") return new Response("This grant changed elsewhere.", { status: 409 })
      return Response.json({ members: [{ ...alice, grants: [{ ...grant, revision: 4 }] }], can_manage_access: true })
    })
    await controller.reload()
    expect(controller.state.action.grant.revision).toBe(3)
    expect(controller.state.action.role).toBe("manager")
    expect(await controller.saveAction()).toBe(false)
    expect(request.mock.calls.findLast(([, init]) => init?.method === "PATCH")?.[1]?.body).toBe(JSON.stringify({ role: "manager", expected_revision: 3 }))
    expect(controller.state.error).toContain("changed elsewhere")
    expect(controller.state.action.role).toBe("manager")
    await controller.reloadAction()
    expect(controller.state.action.grant.revision).toBe(4)
    expect(controller.state.action.role).toBe("member")
    controller.stop()
  })

  it("only revokes on confirmation and cancels a late permission error when the action closes", async () => {
    const { controller, request } = setup()
    await controller.open()
    controller.beginAction("remove", "group", developers.grants[0], "Developers")
    controller.cancelAction()
    expect(request.mock.calls.some(([, init]) => init?.method)).toBe(false)
    let resolve!: (response: Response) => void
    request.mockImplementation((_path, init) => init?.method === "PATCH" ? new Promise((done) => { resolve = done }) : Promise.resolve(Response.json({ members: [], can_manage_access: true })))
    controller.beginAction("remove", "group", developers.grants[0], "Developers")
    const saving = controller.saveAction()
    const mutation = request.mock.calls.findLast(([, init]) => init?.method === "PATCH")![1]!
    expect(mutation.body).toBe(JSON.stringify({ enabled: false, expected_revision: 3 }))
    controller.cancelAction()
    expect(mutation.signal?.aborted).toBe(true)
    resolve(new Response("forbidden", { status: 403 }))
    await saving
    expect(controller.state.canManage).toBe(true)
    expect(controller.state.error).toBe("")
    controller.stop()
  })

  it("rejects stale list responses after changing projects and preserves data on refresh failure", async () => {
    const { controller, request } = setup()
    await controller.open()
    request.mockResolvedValue(new Response("unavailable", { status: 503 }))
    expect(await controller.reload()).toBe(false)
    expect(controller.people.state.items).toEqual([alice])
    let resolve!: (body: unknown) => void
    request.mockImplementation(async () => ({ ok: true, status: 200, json: () => new Promise((done) => { resolve = done }) }) as Response)
    const old = controller.people.reload()
    await vi.waitFor(() => expect(resolve).toBeTypeOf("function"))
    controller.start("workspace", "other-project")
    resolve({ members: [alice], can_manage_access: true })
    await old
    expect(controller.people.state.items).toEqual([])
    expect(controller.state.canManage).toBe(false)
    controller.stop()
  })

  it("reconciles project revocation while closed and disables mutations after manager loss", async () => {
    const { controller, request, refresh, onAccessLost, unsubscribe } = setup()
    await controller.open()
    controller.beginAction("role", "principal", grant, "Alice")
    controller.state.action.role = "manager"
    request.mockResolvedValue(new Response("forbidden", { status: 403 }))
    expect(await controller.saveAction()).toBe(false)
    expect(controller.state.canManage).toBe(false)
    expect(controller.state.error).toContain("no longer have permission")
    controller.close()
    request.mockResolvedValue(new Response("not found", { status: 404 }))
    await refresh()
    expect(onAccessLost).toHaveBeenCalledOnce()
    controller.stop()
    expect(unsubscribe).toHaveBeenCalled()
  })

  it("does not let an older roster response restore permissions after a forbidden mutation", async () => {
    const { controller, request } = setup()
    await controller.open()
    let resolve!: (body: unknown) => void
    request.mockImplementation(async (_path, init) => {
      if (init?.method === "PATCH") return new Response("forbidden", { status: 403 })
      return { ok: true, status: 200, json: () => new Promise((done) => { resolve = done }) } as Response
    })
    const old = controller.people.reload()
    await vi.waitFor(() => expect(resolve).toBeTypeOf("function"))
    controller.beginAction("remove", "principal", grant, "Alice")
    expect(await controller.saveAction()).toBe(false)
    resolve({ members: [alice], can_manage_access: true })
    await old
    expect(controller.state.canManage).toBe(false)
    expect(controller.state.error).toContain("no longer have permission")
    controller.stop()
  })
})
