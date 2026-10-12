import { describe, expect, it, vi } from "vitest"
import { importClientModule } from "../testing/client-module"

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}

describe("settled authentication and access gates", () => {
  it("keeps an authoritative system decision when the workspace request fails", async () => {
    const { createAccess } = await importClientModule(
      new URL("./access.svelte.ts", import.meta.url),
    )
    for (const [status, expected] of [
      [200, "available"],
      [403, "denied"],
    ] as const) {
      const access = createAccess({
        activity: { subscribe: () => () => {} },
        onAuthenticationLost: vi.fn(),
        transport: {
          system: async () => new Response(null, { status }),
          workspaces: async () => {
            throw new Error("offline")
          },
        },
      })
      expect(await access.refresh()).toBe(false)
      expect(access.state.systemAccess).toBe(expected)
      expect(access.state.workspaceStatus).toBe("unavailable")
      access.clear()
    }
  })
  it("retains claims during reload and failure, but rejects a late result after logout", async () => {
    const { createAuth } = await importClientModule(
      new URL("./auth.svelte.ts", import.meta.url),
    )
    const claims = {
      principal: { ref: { id: "principal" } },
      identity: "person",
    }
    const transport = { check: vi.fn(async () => claims), signIn: vi.fn() }
    const auth = createAuth(transport)
    await auth.check()
    const refresh = deferred<typeof claims>()
    transport.check.mockImplementationOnce(() => refresh.promise)
    const pending = auth.check()
    expect(auth.state.status).toBe("authenticated")
    expect(auth.state.reloading).toBe(true)
    refresh.reject(new Error("offline"))
    await pending
    expect(auth.state.claims).toEqual(claims)
    expect(auth.state.status).toBe("authenticated")
    expect(auth.state.reloadError).not.toBe("")
    const old = deferred<typeof claims>()
    transport.check.mockImplementationOnce(() => old.promise)
    const obsolete = auth.check()
    auth.clear()
    old.resolve(claims)
    await obsolete
    expect(auth.state.status).toBe("anonymous")
    expect(auth.state.claims).toBeNull()
  })

  it("keeps workspace forms eligible on transient failure and reports failed reconciliation", async () => {
    const { createAccess } = await importClientModule(
      new URL("./access.svelte.ts", import.meta.url),
    )
    const transport = {
      system: vi.fn(async () => Response.json([])),
      workspaces: vi.fn(async () =>
        Response.json([{ id: "workspace", name: "Workspace" }]),
      ),
    }
    let reconcile!: () => Promise<void>
    const activity = {
      subscribe: vi.fn((_topics, callback) => {
        reconcile = callback
        return () => {}
      }),
    }
    const access = createAccess({
      transport,
      activity,
      onAuthenticationLost: vi.fn(),
    })
    access.start("principal")
    await access.refresh()
    const refresh = deferred<Response>()
    transport.workspaces.mockImplementationOnce(() => refresh.promise)
    const pending = access.refresh()
    expect(access.state.workspaceStatus).toBe("ready")
    expect(access.state.systemAccess).toBe("available")
    refresh.reject(new Error("offline"))
    expect(await pending).toBe(false)
    expect(access.state.workspaces).toEqual([
      { id: "workspace", name: "Workspace" },
    ])
    expect(access.state.workspaceStatus).toBe("ready")
    transport.workspaces.mockRejectedValueOnce(new Error("offline"))
    await expect(reconcile()).rejects.toThrow("access refresh failed")
    transport.system.mockResolvedValueOnce(new Response(null, { status: 403 }))
    await access.refresh()
    expect(access.state.systemAccess).toBe("denied")
  })

  it("does not publish decoded access from a previous principal", async () => {
    const { createAccess } = await importClientModule(
      new URL("./access.svelte.ts", import.meta.url),
    )
    const decoded = deferred<{ id: string }[]>()
    const transport = {
      system: vi.fn(async () => Response.json([])),
      workspaces: vi.fn(async () => ({
        ok: true,
        status: 200,
        json: () => decoded.promise,
      })),
    }
    const access = createAccess({
      transport,
      activity: { subscribe: () => () => {} },
      onAuthenticationLost: vi.fn(),
    })
    access.start("first")
    const pending = access.refresh()
    await vi.waitFor(() => expect(transport.workspaces).toHaveBeenCalled())
    access.start("second")
    expect(access.state.workspaces).toEqual([])
    decoded.resolve([{ id: "old-workspace" }])
    expect(await pending).toBe(false)
    expect(access.state.workspaces).toEqual([])
    expect(access.state.workspaceStatus).toBe("checking")
  })
})
