import { afterEach, describe, expect, it, vi } from "vitest"
import { createInputTransport } from "./input-api"

afterEach(() => vi.unstubAllGlobals())

describe("input capability API", () => {
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

  it("uses the form capability for file create and finish, but only the signed URL for bytes", async () => {
    const request = vi.fn(async () => new Response(null, { status: 204 }))
    vi.stubGlobal("fetch", request)
    const transport = createInputTransport("form-token")
    const file = new File(["data"], "report.txt", { type: "text/plain" })
    await transport.createFile(["attachments"], file)
    await transport.uploadFile("/api/v1/storage?token=storage-token", file)
    await transport.finishFile(["attachments"], "sfi_1")
    expect(request.mock.calls.map(([url]) => url)).toEqual([
      "/api/v1/input/files", "/api/v1/storage?token=storage-token", "/api/v1/input/files/sfi_1/finish",
    ])
    expect(request.mock.calls[0][1].headers.Authorization).toBe("Bearer form-token")
    expect(request.mock.calls[1][1].headers?.Authorization).toBeUndefined()
    expect(request.mock.calls[2][1].headers.Authorization).toBe("Bearer form-token")
  })
})
