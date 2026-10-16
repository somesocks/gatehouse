import { afterEach, describe, expect, it, vi } from "vitest"
import { createPasswordController } from "./password-controller.svelte"

afterEach(() => vi.unstubAllGlobals())

const details = { username: "alice", expires_at: "2026-10-17T12:00:00Z" }
function readyResponse() {
  return Response.json(details)
}

describe("standalone set-password flow", () => {
  it("rejects missing and ambiguous capabilities without fetching", async () => {
    const fetch = vi.fn()
    vi.stubGlobal("fetch", fetch)
    for (const fragment of [
      "",
      "#capability=",
      "#capability=first&capability=second",
    ]) {
      const controller = createPasswordController(fragment)
      await controller.load()
      expect(controller.state.status).toBe("invalid")
      controller.stop()
    }
    expect(fetch).not.toHaveBeenCalled()
  })

  it("uses only the fragment capability, validates confirmation, and saves without a login session", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(readyResponse())
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    vi.stubGlobal("fetch", fetch)
    const controller = createPasswordController(
      "#capability=gh-enc%3Atoken%3Falg%3Dtest%26ver%3D1",
    )
    await controller.load()
    expect(controller.state.status).toBe("ready")
    expect(controller.state.username).toBe("alice")
    expect(fetch.mock.calls[0]).toEqual([
      "/api/v1/auth/set-password",
      expect.objectContaining({
        method: "GET",
        credentials: "omit",
        cache: "no-store",
          headers: { Authorization: "Bearer gh-enc:token?alg=test&ver=1" },
      }),
    ])
    controller.state.password = "new password"
    controller.state.confirmation = "different"
    await controller.save()
    expect(controller.state.error).toMatch(/do not match/)
    expect(fetch).toHaveBeenCalledTimes(1)
    controller.state.confirmation = "new password"
    await controller.save()
    expect(fetch.mock.calls[1]).toEqual([
      "/api/v1/auth/set-password",
      expect.objectContaining({
        method: "POST",
        credentials: "omit",
        body: JSON.stringify({ password: "new password" }),
      }),
    ])
    expect(controller.state.status).toBe("success")
    expect(controller.state.password).toBe("")
    expect(controller.state.confirmation).toBe("")
    await controller.save()
    expect(fetch).toHaveBeenCalledTimes(2)
    controller.stop()
  })

  it.each([401, 409, 410])(
    "handles invalid, used, and expired links (%s)",
    async (status) => {
      const fetch = vi.fn(
        async () =>
          new Response("Ask an administrator for a new link.", { status }),
      )
      vi.stubGlobal("fetch", fetch)
      const controller = createPasswordController("#capability=token")
      await controller.load()
      expect(controller.state.status).toBe("invalid")
      expect(controller.state.error).toMatch(/new link/)
      await controller.save()
      expect(fetch).toHaveBeenCalledTimes(1)
      controller.stop()
    },
  )

  it("handles expiration while the form is open and clears the submitted password", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValueOnce(readyResponse())
        .mockResolvedValueOnce(
          new Response("This link has expired.", { status: 410 }),
        ),
    )
    const controller = createPasswordController("#capability=token")
    await controller.load()
    controller.state.password = controller.state.confirmation = "secret"
    await controller.save()
    expect(controller.state.status).toBe("invalid")
    expect(controller.state.error).toMatch(/expired/)
    expect(controller.state.password).toBe("")
    controller.stop()
  })

  it("allows retrying network failures without retaining submitted passwords", async () => {
    const fetch = vi
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce(readyResponse())
      .mockRejectedValueOnce(new Error("offline"))
    vi.stubGlobal("fetch", fetch)
    const controller = createPasswordController("#capability=token")
    await controller.load()
    expect(controller.state.status).toBe("unavailable")
    await controller.load()
    expect(controller.state.status).toBe("ready")
    controller.state.password = controller.state.confirmation = "secret"
    await controller.save()
    expect(controller.state.status).toBe("ready")
    expect(controller.state.error).toMatch(/could not be saved/)
    expect(controller.state.password).toBe("")
    controller.stop()
  })

  it("aborts requests and ignores late responses after leaving the page", async () => {
    let resolve!: (response: Response) => void
    const fetch = vi.fn<typeof globalThis.fetch>(
      () =>
        new Promise<Response>((done) => {
          resolve = done
        }),
    )
    vi.stubGlobal("fetch", fetch)
    const controller = createPasswordController("#capability=token")
    const pending = controller.load()
    controller.stop()
    expect(fetch.mock.calls[0][1]?.signal?.aborted).toBe(true)
    resolve(readyResponse())
    await pending
    expect(controller.state.username).toBe("")
    expect(controller.state.password).toBe("")
  })
})
