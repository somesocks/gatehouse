import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import { importClientModule } from "../testing/client-module"
import type {
  ActivityClient,
  ActivityRefresh,
  ActivitySelector,
} from "../app/activity"

afterEach(() => vi.unstubAllGlobals())
let client: Awaited<ReturnType<typeof importClientModule>>
beforeAll(async () => {
  client = await importClientModule(
    new URL("./session-participant-add-controller.svelte.ts", import.meta.url),
  )
}, 15000)

const alice = { id: "alice", name: "Alice", enabled: true, grants: [] }
const developers = {
  id: "developers",
  name: "Developers",
  enabled: true,
  grants: [],
}
function setup(activity?: ActivityClient) {
  const request = vi.fn(async (path: string, _init?: RequestInit) =>
    Response.json({
      participants: path.includes("group-options") ? [developers] : [alice],
    }),
  )
  vi.stubGlobal("fetch", request)
  const onAdded = vi.fn(async () => {}),
    onAuthenticationLost = vi.fn(),
    onPermissionsChanged = vi.fn()
  const controller = client.createSessionParticipantAddController({
    activity,
    onAdded,
    onAuthenticationLost,
    onPermissionsChanged,
  })
  return {
    controller,
    request,
    onAdded,
    onAuthenticationLost,
    onPermissionsChanged,
  }
}

describe("add participant child-dialog lifecycle", () => {
  it("refreshes eligibility on membership activity and disposes its chat-scoped subscription", async () => {
    let refresh!: ActivityRefresh
    const unsubscribe = vi.fn()
    const activity = {
      subscribe: vi.fn(
        (_selectors: Iterable<ActivitySelector>, callback: ActivityRefresh) => {
          refresh = callback
          return unsubscribe
        },
      ),
      poll: vi.fn(async () => {}),
      dispose: vi.fn(),
    }
    const { controller, request, onAdded } = setup(activity)
    controller.open("workspace", "chat", "principal")
    await vi.waitFor(() =>
      expect(controller.options.state.items).toHaveLength(1),
    )
    controller.select("alice")
    expect(activity.subscribe.mock.calls[0][0]).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          topic: "workspace",
          events: expect.arrayContaining(["group_member.*", "group_agent.*"]),
        }),
        expect.objectContaining({
          topic: "workspace/chat",
          events: ["session_grant.*"],
        }),
      ]),
    )
    request.mockResolvedValueOnce(Response.json({ participants: [] }))
    const update = {
      names: new Set(["invitation-eligibility"]),
      signal: new AbortController().signal,
    }
    await refresh(update)
    expect(controller.options.state.items).toEqual([])
    expect(controller.state.subject).toBe("alice")
    request.mockResolvedValueOnce(
      new Response("Participant is unavailable", { status: 404 }),
    )
    expect(await controller.save()).toBe(false)
    expect(onAdded).not.toHaveBeenCalled()
    expect(controller.state.error).toContain("unavailable")
    controller.close()
    expect(unsubscribe).toHaveBeenCalledOnce()
    const calls = request.mock.calls.length
    await refresh(update)
    expect(request).toHaveBeenCalledTimes(calls)
  })

  it.each(["principal", "group"] as const)(
    "loads %s options only on opening and submits the chosen subject and role",
    async (kind) => {
      const { controller, request, onAdded } = setup()
      expect(request).not.toHaveBeenCalled()
      const close = controller.open("workspace", "chat", kind)
      await vi.waitFor(() =>
        expect(controller.options.state.items).toHaveLength(1),
      )
      expect(request.mock.calls[0][0]).toContain(`/${kind}-options?`)
      const item = controller.options.state.items[0]
      controller.select(item.id)
      controller.state.role = "manager"
      expect(await controller.save()).toBe(true)
      expect(JSON.parse(request.mock.calls.at(-1)![1]!.body as string)).toEqual(
        { [kind]: item.id, role: "manager" },
      )
      expect(onAdded).toHaveBeenCalledTimes(1)
      close()
    },
  )

  it("keeps the selected person and role after a failed add or options refresh", async () => {
    const { controller, request, onAdded } = setup()
    controller.open("workspace", "chat", "principal")
    await vi.waitFor(() =>
      expect(controller.options.state.items).toHaveLength(1),
    )
    controller.select("alice")
    controller.state.role = "member"
    request.mockResolvedValueOnce(
      new Response("This role is already assigned.", { status: 409 }),
    )
    expect(await controller.save()).toBe(false)
    expect(controller.state.subject).toBe("alice")
    expect(controller.state.selectedName).toBe("Alice")
    expect(controller.state.role).toBe("member")
    expect(controller.state.error).toContain("already assigned")
    expect(onAdded).not.toHaveBeenCalled()
    request.mockResolvedValueOnce(new Response(null, { status: 503 }))
    await controller.options.search()
    expect(controller.options.state.items).toEqual([alice])
    expect(controller.state.subject).toBe("alice")
    controller.close()
  })

  it("aborts pending work on cancel and ignores an old save after opening another child dialog", async () => {
    const { controller, request, onAdded } = setup()
    controller.open("workspace", "chat", "principal")
    await vi.waitFor(() =>
      expect(controller.options.state.items).toHaveLength(1),
    )
    controller.select("alice")
    let finish!: (response: Response) => void
    request.mockImplementationOnce(
      async () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const pending = controller.save()
    const signal = request.mock.calls.at(-1)![1]!.signal!
    controller.close()
    expect(signal.aborted).toBe(true)
    controller.open("workspace", "other-chat", "group")
    await vi.waitFor(() =>
      expect(controller.options.state.items[0]?.id).toBe("developers"),
    )
    controller.select("developers")
    finish(Response.json({ id: "old-grant" }))
    expect(await pending).toBe(false)
    expect(onAdded).not.toHaveBeenCalled()
    expect(controller.state.subject).toBe("developers")
    expect(controller.state.saving).toBe(false)
    controller.close()
  })

  it("ignores late option decoding after the child dialog is cancelled", async () => {
    const { controller, request } = setup()
    let decode!: (body: unknown) => void
    request.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () =>
        new Promise((resolve) => {
          decode = resolve
        }),
    } as Response)
    controller.open("workspace", "chat", "principal")
    await vi.waitFor(() => expect(decode).toBeTypeOf("function"))
    controller.close()
    decode({ participants: [alice] })
    await vi.waitFor(() => expect(controller.options.state.busy).toBe(false))
    expect(controller.options.state.items).toEqual([])
    expect(controller.state.open).toBe(false)
  })
})
