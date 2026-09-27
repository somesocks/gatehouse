import { afterEach, describe, expect, it, vi } from "vitest"
import { checkAuthentication, signIn } from "./auth"
import { clearAccessToken } from "./api"

afterEach(() => {
  clearAccessToken()
  vi.unstubAllGlobals()
})

describe("authentication transport", () => {
  it("returns null for an anonymous session", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(null, { status: 401 })),
    )

    await expect(checkAuthentication()).resolves.toBeNull()
  })

  it("returns claims for an authenticated session", async () => {
    const claims = { principal: { ref: { id: "prn_1" } }, identity: "user" }
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => Response.json(claims)),
    )

    await expect(checkAuthentication()).resolves.toEqual(claims)
  })

  it("reports invalid credentials without throwing", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(null, { status: 401 })),
    )

    await expect(signIn("user", "wrong")).resolves.toBe(false)
  })

  it("uses the login response bearer when cookies are unavailable", async () => {
    const claims = { principal: { ref: { id: "prn_1" } }, identity: "user" }
    const request = vi.fn(async (path: string, _options?: RequestInit) =>
      path === "/api/v1/auth/login"
        ? Response.json({ access_token: "memory-token", token_type: "Bearer" })
        : Response.json(claims),
    )
    vi.stubGlobal("fetch", request)
    await expect(signIn("user", "password")).resolves.toBe(true)
    await expect(checkAuthentication()).resolves.toEqual(claims)
    const options = request.mock.calls[1][1] as RequestInit
    expect((options.headers as Headers).get("Authorization")).toBe("Bearer memory-token")
  })
})
