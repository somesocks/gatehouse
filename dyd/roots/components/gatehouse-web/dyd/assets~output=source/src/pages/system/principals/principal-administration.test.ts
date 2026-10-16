import { afterEach, describe, expect, it, vi } from "vitest"
import { clearAccessToken } from "../../../app/api"
import { parseRoute, routePath } from "../../../route"
import { routeOwner } from "../../../route-owner"
import { createPrincipalNewController } from "./principal-new-controller.svelte"
import { createPrincipalLoginsController } from "./principal-logins-controller.svelte"

afterEach(() => {
  clearAccessToken()
  vi.unstubAllGlobals()
})
const principal = {
  id: "prn_account",
  enabled: true,
  revision: 1,
  identities: [],
}
const login = {
  id: "idt_login",
  key: "gatehouse:user",
  enabled: true,
  revision: 3,
}
function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}
function fixture() {
  const onChanged = vi.fn(),
    onAuthenticationLost = vi.fn(),
    onReload = vi.fn(async () => {})
  const controller = createPrincipalLoginsController({
    onChanged,
    onAuthenticationLost,
    onReload,
    onSystemAccessChange: vi.fn(),
    currentIdentityID: () => login.id,
  })
  controller.reset(principal.id)
  vi.stubGlobal("window", { confirm: vi.fn(() => true) })
  return { controller, onChanged, onAuthenticationLost, onReload }
}

describe("principal administration", () => {
  it("routes principal creation separately from principal details", () => {
    const route = parseRoute(
      new URL("http://localhost/app/system/principals/new"),
    )
    expect(route).toEqual({ kind: "system-principal-new" })
    expect(routeOwner(route)).toBe("principal-new")
    expect(routePath({ kind: "system-principal-new" })).toBe(
      "/app/system/principals/new",
    )
  })
  it("creates an account without credentials and reports alias conflicts", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce(response(principal))
      .mockResolvedValueOnce(new Response("alias in use", { status: 409 }))
    vi.stubGlobal("fetch", request)
    const controller = createPrincipalNewController({
      onAuthenticationLost: vi.fn(),
      onSystemAccessChange: vi.fn(),
    })
    controller.state.name = " Account "
    controller.state.alias = "account"
    expect(await controller.create()).toEqual(principal)
    expect(JSON.parse(request.mock.calls[0][1].body)).toEqual({
      name: "Account",
      alias: "account",
      enabled: true,
    })
    expect(await controller.create()).toBeNull()
    expect(controller.state.error).toMatch(/alias.*already in use/)
    controller.stop()
  })
  it("adds a login, validates password confirmation, and clears submitted passwords", async () => {
    const request = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        response(principal),
    )
    vi.stubGlobal("fetch", request)
    const { controller, onChanged } = fixture()
    controller.begin("add")
    controller.state.username = "new-user"
    controller.state.password = "secret"
    expect(await controller.save()).toBe(false)
    expect(request).not.toHaveBeenCalled()
    controller.state.confirmation = "secret"
    expect(await controller.save()).toBe(true)
    expect(request.mock.calls[0]).toEqual([
      expect.stringContaining(`/principals/${principal.id}/identities`),
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          username: "new-user",
          enabled: true,
          password: "secret",
        }),
      }),
    ])
    expect(onChanged).toHaveBeenCalledWith(principal)
    expect(controller.state.password).toBe("")
    expect(controller.state.mode).toBeNull()
    controller.stop()
  })
  it("renames without sending a password and resets passwords without signing out", async () => {
    const request = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        response(principal),
    )
    vi.stubGlobal("fetch", request)
    const { controller, onAuthenticationLost } = fixture()
    controller.begin("edit", login)
    controller.state.username = "renamed"
    await controller.save()
    expect(JSON.parse(String(request.mock.calls[0][1]?.body))).toEqual({
      username: "renamed",
      enabled: true,
      expected_revision: 3,
    })
    controller.begin("password", login)
    controller.state.password = controller.state.confirmation = "replacement"
    await controller.save()
    expect(JSON.parse(String(request.mock.calls[1][1]?.body))).toEqual({
      password: "replacement",
      expected_revision: 3,
    })
    expect(controller.state.notice).toMatch(/sessions remain active/)
    expect(onAuthenticationLost).not.toHaveBeenCalled()
    controller.stop()
  })
  it("preserves non-secret drafts on conflicts and refreshes stale revisions", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("revision conflict", { status: 409 })),
    )
    const { controller, onChanged, onReload } = fixture()
    controller.begin("edit", login)
    controller.state.username = "draft"
    expect(await controller.save()).toBe(false)
    expect(controller.state.username).toBe("draft")
    expect(controller.state.conflict).toBe(true)
    expect(onReload).toHaveBeenCalledOnce()
    expect(onChanged).not.toHaveBeenCalled()
    controller.cancel()
    controller.begin("edit", { ...login, revision: 4 })
    expect(controller.state.conflict).toBe(false)
    expect(controller.state.identity?.revision).toBe(4)
    controller.stop()
  })
  it("invalidates the current login after disabling or deleting it", async () => {
    const request = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        response(principal),
    )
    vi.stubGlobal("fetch", request)
    const { controller, onAuthenticationLost } = fixture()
    await controller.setEnabled(login)
    expect(JSON.parse(String(request.mock.calls[0][1]?.body))).toEqual({
      enabled: false,
      expected_revision: 3,
    })
    expect(onAuthenticationLost).toHaveBeenCalledOnce()
    await controller.remove(login)
    expect(request.mock.calls[1][1]?.method).toBe("DELETE")
    expect(onAuthenticationLost).toHaveBeenCalledTimes(2)
    controller.stop()
  })
  it("ignores late responses and preserves a new principal's draft", async () => {
    let resolve!: (value: Response) => void
    vi.stubGlobal(
      "fetch",
      vi.fn(
        () =>
          new Promise<Response>((done) => {
            resolve = done
          }),
      ),
    )
    const { controller, onChanged } = fixture()
    controller.begin("password", login)
    controller.state.password = controller.state.confirmation = "old-secret"
    const pending = controller.save()
    controller.reset("prn_other")
    controller.begin("add")
    controller.state.password = "new-draft"
    resolve(response(principal))
    expect(await pending).toBe(false)
    expect(onChanged).not.toHaveBeenCalled()
    expect(controller.state.password).toBe("new-draft")
    controller.stop()
    expect(controller.state.password).toBe("")
  })
  it("creates and copies a link without changing the identity, then clears it when leaving", async () => {
    const link = {
      url: "https://gatehouse.test/app/set-password/#capability=secret",
      expires_at: "2026-10-17T12:00:00Z",
    }
    const request = vi.fn(async () => response(link))
    const writeText = vi.fn(async () => {})
    vi.stubGlobal("fetch", request)
    vi.stubGlobal("navigator", { clipboard: { writeText } })
    const { controller, onChanged } = fixture()
    await controller.createPasswordLink(login)
    expect(request.mock.calls[0]).toEqual([
      expect.stringContaining(
        `/principals/${principal.id}/identities/${login.id}/password-link`,
      ),
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ expected_revision: login.revision }),
      }),
    ])
    expect(controller.state.passwordLink).toEqual({
      ...link,
      username: "user",
    })
    expect(onChanged).not.toHaveBeenCalled()
    await controller.copyPasswordLink()
    expect(writeText).toHaveBeenCalledWith(link.url)
    expect(controller.state.notice).toMatch(/copied/)
    controller.stop()
    expect(controller.state.passwordLink).toBeNull()
  })
  it("reloads changed identities and ignores link responses for another principal", async () => {
    let resolve!: (response: Response) => void
    const request = vi
      .fn()
      .mockResolvedValueOnce(new Response("revision conflict", { status: 409 }))
      .mockImplementationOnce(
        () =>
          new Promise<Response>((done) => {
            resolve = done
          }),
      )
    vi.stubGlobal("fetch", request)
    const { controller, onReload } = fixture()
    await controller.createPasswordLink(login)
    expect(onReload).toHaveBeenCalledOnce()
    expect(controller.state.error).toMatch(/changed/)
    const pending = controller.createPasswordLink(login)
    controller.reset("prn_other")
    resolve(response({ url: "secret", expires_at: "later" }))
    await pending
    expect(controller.state.passwordLink).toBeNull()
    controller.stop()
  })
})
