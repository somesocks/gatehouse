import { afterEach, describe, expect, it, vi } from "vitest"
import { createSessionTask, fetchSessionTask, fetchSessionTasks, removeSessionTask, updateSessionTask } from "./session-tasks"

afterEach(() => vi.unstubAllGlobals())

describe("session tasks transport", () => {
  it("uses encoded resource paths, same-origin credentials, and route cancellation", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const controller = new AbortController()
    const input = { title: "A task", description: "Description", sensitive: true, status: "ready" as const }
    await fetchSessionTasks("wsp/test", "ses/test", controller.signal)
    await fetchSessionTask("wsp/test", "ses/test", "task/test", controller.signal)
    await createSessionTask("wsp/test", "ses/test", input, controller.signal)
    await updateSessionTask("wsp/test", "ses/test", "task/test", input, controller.signal)
    await removeSessionTask("wsp/test", "ses/test", "task/test", controller.signal)
    expect(fetch).toHaveBeenNthCalledWith(1, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/tasks", { credentials: "same-origin", signal: controller.signal })
    expect(fetch).toHaveBeenNthCalledWith(2, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/tasks/task%2Ftest", { credentials: "same-origin", signal: controller.signal })
    expect(fetch).toHaveBeenNthCalledWith(3, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/tasks", expect.objectContaining({ method: "POST", credentials: "same-origin", signal: controller.signal, body: JSON.stringify(input) }))
    expect(fetch).toHaveBeenNthCalledWith(4, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/tasks/task%2Ftest", expect.objectContaining({ method: "PATCH", credentials: "same-origin", signal: controller.signal, body: JSON.stringify(input) }))
    expect(fetch).toHaveBeenNthCalledWith(5, "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/tasks/task%2Ftest", { method: "DELETE", credentials: "same-origin", signal: controller.signal })
  })
})
