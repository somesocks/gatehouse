import { describe, expect, it } from "vitest"
import { loginDestination } from "./login-destination"

describe("login destination", () => {
  const origin = "https://gatehouse.example"

  it("uses the application home when no next destination is supplied", () => {
    expect(loginDestination(null, origin)).toBe("/app/")
  })

  it("rejects malformed, external, and login-loop destinations", () => {
    expect(loginDestination("http://[invalid", origin)).toBe("/app/")
    expect(loginDestination("https://example.com/app/", origin)).toBe("/app/")
    expect(loginDestination("/app/login", origin)).toBe("/app/")
    expect(loginDestination("/app/login/", origin)).toBe("/app/")
  })

  it("preserves same-origin application paths, query strings, and hashes", () => {
    expect(loginDestination("/app/wsp/wsp_a/ses?name=chat#latest", origin)).toBe("/app/wsp/wsp_a/ses?name=chat#latest")
  })
})
