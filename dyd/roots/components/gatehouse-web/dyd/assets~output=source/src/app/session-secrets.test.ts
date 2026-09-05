import { afterEach, describe, expect, it, vi } from "vitest"
import { createSessionSecret, fetchSessionSecret, fetchSessionSecrets, removeSessionSecret, updateSessionSecret } from "./session-secrets"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("session secrets transport", () => {
  it("uses encoded resource paths and session credentials", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)

    await fetchSessionSecrets("wsp/test", "ses/test")
    await fetchSessionSecret("wsp/test", "ses/test", "sec/test")
    await createSessionSecret("wsp/test", "ses/test", { description: "A secret", value: "value" })
    await updateSessionSecret("wsp/test", "ses/test", "sec/test", { description: "Renamed" })
    await removeSessionSecret("wsp/test", "ses/test", "sec/test")

    expect(fetch).toHaveBeenNthCalledWith(1, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/secrets", { credentials: "same-origin" })
    expect(fetch).toHaveBeenNthCalledWith(2, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/secrets/sec%2Ftest", { credentials: "same-origin" })
    expect(fetch).toHaveBeenNthCalledWith(3, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/secrets", expect.objectContaining({ method: "POST", credentials: "same-origin" }))
    expect(fetch).toHaveBeenNthCalledWith(4, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/secrets/sec%2Ftest", expect.objectContaining({ method: "PATCH", credentials: "same-origin" }))
    expect(fetch).toHaveBeenNthCalledWith(5, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/secrets/sec%2Ftest", { method: "DELETE", credentials: "same-origin" })
  })
})
