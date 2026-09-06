import { afterEach, describe, expect, it, vi } from "vitest"
import { createSessionSecret, fetchSessionSecret, fetchSessionSecrets, removeSessionSecret, updateSessionSecret } from "./session-secrets"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("session secrets transport", () => {
  it("uses encoded resource paths, session credentials, and route cancellation", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const controller = new AbortController()

    await fetchSessionSecrets("wsp/test", "ses/test", controller.signal)
    await fetchSessionSecret("wsp/test", "ses/test", "sec/test", controller.signal)
    await createSessionSecret("wsp/test", "ses/test", { description: "A secret", value: "value" }, controller.signal)
    await updateSessionSecret("wsp/test", "ses/test", "sec/test", { description: "Renamed" }, controller.signal)
    await removeSessionSecret("wsp/test", "ses/test", "sec/test", controller.signal)

    expect(fetch).toHaveBeenNthCalledWith(1, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/secrets", { credentials: "same-origin", signal: controller.signal })
    expect(fetch).toHaveBeenNthCalledWith(2, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/secrets/sec%2Ftest", { credentials: "same-origin", signal: controller.signal })
    expect(fetch).toHaveBeenNthCalledWith(3, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/secrets", expect.objectContaining({ method: "POST", credentials: "same-origin", signal: controller.signal }))
    expect(fetch).toHaveBeenNthCalledWith(4, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/secrets/sec%2Ftest", expect.objectContaining({ method: "PATCH", credentials: "same-origin", signal: controller.signal }))
    expect(fetch).toHaveBeenNthCalledWith(5, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/secrets/sec%2Ftest", { method: "DELETE", credentials: "same-origin", signal: controller.signal })
  })
})
