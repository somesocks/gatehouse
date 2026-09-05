import { afterEach, describe, expect, it, vi } from "vitest"
import { fetchProjectSecret, fetchProjectSecrets } from "./project-secrets"

afterEach(() => vi.unstubAllGlobals())

describe("project secrets transport", () => {
  it("uses encoded project-secret paths", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    await fetchProjectSecrets("wsp/test", "prj/test")
    await fetchProjectSecret("wsp/test", "prj/test", "sec/test")
    expect(fetch).toHaveBeenNthCalledWith(1, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/secrets", { credentials: "same-origin" })
    expect(fetch).toHaveBeenNthCalledWith(2, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/secrets/sec%2Ftest", { credentials: "same-origin" })
  })
})
