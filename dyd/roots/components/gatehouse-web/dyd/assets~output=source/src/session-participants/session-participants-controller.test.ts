import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import { importClientModule } from "../testing/client-module"
import type { ActivityRefresh, ActivitySelector } from "../app/activity"

afterEach(() => vi.unstubAllGlobals())

let client: Awaited<ReturnType<typeof importClientModule>>
// Cold-compiling the real rune module belongs to setup, rather than the first
// behavior test's timeout budget when the full suite is running concurrently.
beforeAll(async () => {
  client = await importClientModule(
    new URL("./session-participants-controller.svelte.ts", import.meta.url),
  )
}, 15000)

const grant = {
  id: "grant",
  principal: "alice",
  role: "contributor",
  enabled: true,
  revision: 3,
}
const alice = { id: "alice", name: "Alice", enabled: true, grants: [grant] }
const developers = {
  id: "developers",
  name: "Developers",
  enabled: true,
  grants: [
    { ...grant, id: "group-grant", principal: undefined, group: "developers" },
  ],
}
const assistant = {
  id: "assistant",
  alias: "assistant",
  enabled: true,
  grants: [
    { ...grant, id: "bot-grant", principal: undefined, agent: "assistant" },
  ],
}

async function setup(canManage = true) {
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
  const onAuthenticationLost = vi.fn(),
    onAccessLost = vi.fn()
  const request = vi.fn(async (path: string, _init?: RequestInit) => {
    if (!path.includes("/participants")) return Response.json({ id: "chat" })
    const kind = new URL(path, "http://localhost").searchParams.get("kind")
    return Response.json({
      participants:
        kind === "group"
          ? [developers]
          : kind === "agent"
            ? [assistant]
            : [alice],
      can_manage_participants: canManage,
    })
  })
  vi.stubGlobal("fetch", request)
  const { createSessionParticipantsController } = client
  const controller = createSessionParticipantsController({
    activity,
    onAuthenticationLost,
    onAccessLost,
  })
  controller.start("workspace", "chat")
  return {
    controller,
    request,
    activity,
    unsubscribe,
    onAuthenticationLost,
    onAccessLost,
    refresh: () =>
      refresh({
        names: new Set(["chat-participants"]),
        signal: new AbortController().signal,
      }),
  }
}

describe("chat participant lifecycle", () => {
  it("keeps inherited participation outside the editable bot grants", async () => {
    const { controller, request } = await setup()
    const inherited = {
      ...assistant,
      grants: [],
      inherited_from: [{ group: "developers", role: "contributor" }],
    }
    request.mockImplementation(async (path: string) => {
      const kind = new URL(path, "http://localhost").searchParams.get("kind")
      return Response.json({
        participants: kind === "agent" ? [inherited] : [],
        can_manage_participants: true,
      })
    })
    await controller.open()
    expect(controller.state.canManage).toBe(true)
    expect(controller.bots.state.items).toEqual([inherited])
    expect(controller.bots.state.items.flatMap((bot) => bot.grants)).toEqual([])
    controller.stop()
  })

  it("loads explicit people, groups, and bots lazily and prevents read-only mutations", async () => {
    const { controller, request } = await setup(false)
    expect(request).not.toHaveBeenCalled()
    await controller.open()
    expect(controller.people.state.items).toEqual([alice])
    expect(controller.groups.state.items).toEqual([developers])
    expect(controller.bots.state.items).toEqual([assistant])
    expect(controller.state.canManage).toBe(false)
    expect(await controller.remove(grant)).toBe(false)
    expect(request).toHaveBeenCalledTimes(3)
    controller.stop()
  })

  it("retains the roster on a failed role edit and sends the grant revision", async () => {
    const { controller, request } = await setup()
    await controller.open()
    request.mockResolvedValueOnce(
      new Response("The role changed elsewhere.", { status: 409 }),
    )
    expect(await controller.update(grant, "manager")).toBe(false)
    expect(controller.people.state.items).toEqual([alice])
    expect(controller.state.error).toContain("changed elsewhere")
    expect(JSON.parse(request.mock.calls.at(-1)![1]!.body as string)).toEqual({
      role: "manager",
      expected_revision: 3,
    })
    request.mockResolvedValueOnce(
      Response.json({ ...grant, revision: 4, enabled: false }),
    )
    expect(await controller.remove(grant)).toBe(true)
    const mutation = request.mock.calls.findLast(
      ([path, init]) =>
        path.endsWith("/grants/grant") && init?.method === "PATCH",
    )!
    expect(JSON.parse(mutation[1]!.body as string)).toEqual({
      enabled: false,
      expected_revision: 3,
    })
    expect(controller.people.state.items[0].id).toBe("alice")
    expect(controller.groups.state.items[0].id).toBe("developers")
    controller.stop()
  })

  it("ignores a late roster decode after changing chats and retains settled data on reload failure", async () => {
    const { controller, request, onAccessLost } = await setup()
    await controller.open()
    let decode!: (body: unknown) => void
    request.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () =>
        new Promise((resolve) => {
          decode = resolve
        }),
    } as Response)
    const old = controller.people.reload()
    await vi.waitFor(() => expect(decode).toBeTypeOf("function"))
    controller.start("workspace", "other-chat")
    await controller.open()
    decode({
      participants: [{ ...alice, id: "obsolete" }],
      can_manage_participants: false,
    })
    expect(await old).toBe(false)
    expect(controller.people.state.items[0].id).toBe("alice")
    expect(controller.state.canManage).toBe(true)
    request.mockResolvedValueOnce(new Response(null, { status: 503 }))
    expect(await controller.people.reload()).toBe(false)
    expect(controller.people.state.items[0].id).toBe("alice")
    expect(onAccessLost).not.toHaveBeenCalled()
    controller.stop()
  })

  it("aborts dialog work on close and ignores an old permission failure after reopening", async () => {
    const { controller, request } = await setup()
    await controller.open()
    let finish!: (response: Response) => void
    request.mockImplementationOnce(
      async () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const pending = controller.remove(grant)
    const signal = request.mock.calls.at(-1)![1]!.signal!
    controller.close()
    expect(signal.aborted).toBe(true)
    await controller.open()
    finish(new Response("forbidden", { status: 403 }))
    expect(await pending).toBe(false)
    expect(controller.state.canManage).toBe(true)
    expect(controller.state.error).toBe("")
    expect(controller.state.saving).toBe(false)
    controller.stop()
  })

  it("cancels a child action without closing the participant list", async () => {
    const { controller, request } = await setup()
    await controller.open()
    let finish!: (response: Response) => void
    request.mockImplementationOnce(
      async () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const pending = controller.remove(grant)
    const signal = request.mock.calls.at(-1)![1]!.signal!
    controller.cancelMutation()
    expect(signal.aborted).toBe(true)
    expect(controller.state.open).toBe(true)
    expect(controller.state.saving).toBe(false)
    finish(new Response("forbidden", { status: 403 }))
    expect(await pending).toBe(false)
    expect(controller.state.canManage).toBe(true)
    expect(controller.people.state.items).toEqual([alice])
    controller.stop()
  })

  it("reconciles revocation while closed, retries transient failure, and disposes subscriptions", async () => {
    const {
      controller,
      request,
      refresh,
      onAccessLost,
      activity,
      unsubscribe,
    } = await setup()
    expect(activity.subscribe.mock.calls[0][0]).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          topic: "workspace/chat",
          events: ["session_grant.*"],
        }),
        expect.objectContaining({
          topic: "workspace",
          events: [
            "group.*",
            "group_member.*",
            "group_agent.*",
            "workspace_agent.*",
          ],
        }),
      ]),
    )
    request.mockResolvedValueOnce(new Response(null, { status: 503 }))
    await expect(refresh()).rejects.toThrow(
      "Chat access could not be refreshed",
    )
    expect(onAccessLost).not.toHaveBeenCalled()
    request.mockResolvedValueOnce(new Response(null, { status: 404 }))
    await refresh()
    expect(onAccessLost).toHaveBeenCalledTimes(1)
    controller.stop()
    expect(unsubscribe).toHaveBeenCalledTimes(1)
  })
})
