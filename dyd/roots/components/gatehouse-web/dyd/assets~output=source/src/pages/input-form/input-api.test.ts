import { afterEach, describe, expect, it, vi } from "vitest"
import { capabilityFromFragment, createInputTransport } from "./input-api"

afterEach(() => vi.unstubAllGlobals())

describe("input capability API", () => {
  it("decodes a single capability fragment", () => {
    expect(capabilityFromFragment("#capability=gh-enc%3Atest%3Fkey%3Done%26ver%3D1")).toBe("gh-enc:test?key=one&ver=1")
    expect(capabilityFromFragment("#capability=one&capability=two")).toBeNull()
    expect(capabilityFromFragment("#missing=one")).toBeNull()
  })

  it("uses only the field form capability without login cookies or a request body on terminal actions", async () => {
    const request = vi.fn(async (_path: string, _options?: RequestInit) => new Response(null, { status: 204 }))
    vi.stubGlobal("fetch", request)
    const transport = createInputTransport("field+token")
    await transport.read()
    await transport.patch('{"op":"remove","path":["name"]}')
    await transport.terminal("submit")
    expect(request.mock.calls.map(([path]) => path)).toEqual([
      "/api/v1/input", "/api/v1/input/draft", "/api/v1/input/submit",
    ])
    for (const [, options] of request.mock.calls) {
      expect(options.credentials).toBe("omit")
      expect(options.headers.Authorization).toBe("Bearer field+token")
    }
    expect(request.mock.calls[2][1].body).toBeUndefined()
  })
})
