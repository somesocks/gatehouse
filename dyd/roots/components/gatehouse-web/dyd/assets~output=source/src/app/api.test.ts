import { afterEach, describe, expect, it, vi } from "vitest"
import { clearAccessToken, fetchGatehouse, setAccessToken } from "./api"

afterEach(() => {
  clearAccessToken()
  vi.unstubAllGlobals()
})

describe("cookie-less Gatehouse API access", () => {
  it("keeps the login bearer in page memory and sends it only to Gatehouse API paths", async () => {
    const request = vi.fn(async (_path: string, _options?: RequestInit) => new Response(null, { status: 204 }))
    vi.stubGlobal("fetch", request)
    setAccessToken("login-bearer")
    await fetchGatehouse("/api/v1/workspaces", { credentials: "same-origin" })
    expect((request.mock.calls[0][1] as RequestInit).headers).toEqual(new Headers({ Authorization: "Bearer login-bearer" }))
    await fetchGatehouse("https://storage.example.test/upload", { method: "PUT" })
    expect(request.mock.calls[1][1]).toEqual({ method: "PUT" })
    clearAccessToken()
    await fetchGatehouse("/api/v1/workspaces", { credentials: "same-origin" })
    expect(request.mock.calls[2][1]).toEqual({ credentials: "same-origin" })
  })
})
