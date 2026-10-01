import { afterEach, describe, expect, it, vi } from "vitest"
import { createFieldTransport, fieldLaunchFromFragment } from "./api"
import { fileBrowseFromSearch } from "./browse"

afterEach(() => vi.unstubAllGlobals())

describe("session file picker transport", () => {
  it("accepts the field bootstrap only for the same-origin field API", () => {
    const fragment = "#" + new URLSearchParams({ version: "1", api: "https://gatehouse.test/api/v1/input/field", capability: "field-token" })
    expect(fieldLaunchFromFragment(fragment, "https://gatehouse.test")).toEqual({ api: "https://gatehouse.test/api/v1/input/field", capability: "field-token" })
    expect(fieldLaunchFromFragment(fragment, "https://other.test")).toBeNull()
    expect(fieldLaunchFromFragment(fragment + "&capability=other", "https://gatehouse.test")).toBeNull()
    expect(fieldLaunchFromFragment(fragment.replace("version=1", "version=2"), "https://gatehouse.test")).toBeNull()
  })

  it("uses the field token for API calls and the storage URL alone for uploading bytes", async () => {
    const request = vi.fn(async (_url: string, _options: RequestInit) => new Response(null, { status: 204 }))
    vi.stubGlobal("fetch", request)
    const transport = createFieldTransport({ api: "https://gatehouse.test/api/v1/input/field", capability: "field-token" })
    const file = new File(["data"], "report.txt", { type: "text/plain" })
    await transport.files(fileBrowseFromSearch("?name=report&limit=10&cursor=sfi_cursor"), ["text/plain"])
    await transport.createFile(file)
    await transport.upload("/api/v1/storage?token=storage-token", file)
    await transport.finishFile("sfi_one")
    await transport.save(["sfi_one", "sfi_two"])
    for (const [index, [, options]] of request.mock.calls.entries()) {
      expect(options.credentials).toBe("omit")
      expect((options.headers as Record<string,string> | undefined)?.Authorization).toBe(index === 2 ? undefined : "Bearer field-token")
    }
    expect(request.mock.calls[0][0]).toBe("https://gatehouse.test/api/v1/input/field/files?name=report&limit=10&cursor=sfi_cursor&accept=text%2Fplain")
    expect(request.mock.calls[4][1].body).toBe('{"op":"set","value":{"file_ids":["sfi_one","sfi_two"]}}')
  })
})
