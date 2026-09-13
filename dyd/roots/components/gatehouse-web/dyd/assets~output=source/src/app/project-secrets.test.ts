import { afterEach, describe, expect, it, vi } from "vitest"
import {
  createProjectSecret,
  fetchProjectSecret,
  fetchProjectSecrets,
  removeProjectSecret,
  updateProjectSecret,
} from "./project-secrets"

afterEach(() => vi.unstubAllGlobals())

describe("project secrets transport", () => {
  it("uses encoded paths, same-origin credentials, and cancellation signals", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const controller = new AbortController()
    await fetchProjectSecrets("wsp/test", "prj/test", controller.signal)
    await fetchProjectSecret(
      "wsp/test",
      "prj/test",
      "sec/test",
      controller.signal,
    )
    await createProjectSecret(
      "wsp/test",
      "prj/test",
      { description: "Secret", value: "value" },
      controller.signal,
    )
    await updateProjectSecret(
      "wsp/test",
      "prj/test",
      "sec/test",
      { description: "Secret" },
      controller.signal,
    )
    await removeProjectSecret(
      "wsp/test",
      "prj/test",
      "sec/test",
      controller.signal,
    )
    expect(fetch).toHaveBeenNthCalledWith(
      1,
      "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/secrets",
      { credentials: "same-origin", signal: controller.signal },
    )
    expect(fetch).toHaveBeenNthCalledWith(
      2,
      "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/secrets/sec%2Ftest",
      { credentials: "same-origin", signal: controller.signal },
    )
    expect(fetch).toHaveBeenNthCalledWith(
      3,
      "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/secrets",
      expect.objectContaining({
        method: "POST",
        credentials: "same-origin",
        signal: controller.signal,
      }),
    )
    expect(fetch).toHaveBeenNthCalledWith(
      4,
      "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/secrets/sec%2Ftest",
      expect.objectContaining({
        method: "PATCH",
        credentials: "same-origin",
        signal: controller.signal,
      }),
    )
    expect(fetch).toHaveBeenNthCalledWith(
      5,
      "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/secrets/sec%2Ftest",
      {
        method: "DELETE",
        credentials: "same-origin",
        signal: controller.signal,
      },
    )
  })
})
