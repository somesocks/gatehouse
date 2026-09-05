import { afterEach, describe, expect, it, vi } from "vitest"
import { checkAuthentication, signIn } from "./auth"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("authentication transport", () => {
  it("returns null for an anonymous session", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 401 })))

    await expect(checkAuthentication()).resolves.toBeNull()
  })

  it("returns claims for an authenticated session", async () => {
    const claims = { principal: { ref: { id: "prn_1" } }, identity: "user" }
    vi.stubGlobal("fetch", vi.fn(async () => Response.json(claims)))

    await expect(checkAuthentication()).resolves.toEqual(claims)
  })

  it("reports invalid credentials without throwing", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 401 })))

    await expect(signIn("user", "wrong")).resolves.toBe(false)
  })
})
