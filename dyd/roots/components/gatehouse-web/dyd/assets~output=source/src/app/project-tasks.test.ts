import { afterEach, describe, expect, it, vi } from "vitest"
import { createProjectTask, fetchProjectTask, fetchProjectTasks, removeProjectTask, updateProjectTask } from "./project-tasks"

afterEach(() => vi.unstubAllGlobals())

describe("project tasks transport", () => {
  it("uses encoded resource paths and same-origin credentials", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const input = { title: "A task", description: "Description", sensitive: true, status: "ready" as const }
    await fetchProjectTasks("wsp/test", "prj/test")
    await fetchProjectTask("wsp/test", "prj/test", "task/test")
    await createProjectTask("wsp/test", "prj/test", input)
    await updateProjectTask("wsp/test", "prj/test", "task/test", input)
    await removeProjectTask("wsp/test", "prj/test", "task/test")
    expect(fetch).toHaveBeenNthCalledWith(1, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/tasks", { credentials: "same-origin" })
    expect(fetch).toHaveBeenNthCalledWith(2, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/tasks/task%2Ftest", { credentials: "same-origin" })
    expect(fetch).toHaveBeenNthCalledWith(3, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/tasks", expect.objectContaining({ method: "POST", credentials: "same-origin", body: JSON.stringify(input) }))
    expect(fetch).toHaveBeenNthCalledWith(4, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/tasks/task%2Ftest", expect.objectContaining({ method: "PATCH", credentials: "same-origin", body: JSON.stringify(input) }))
    expect(fetch).toHaveBeenNthCalledWith(5, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/tasks/task%2Ftest", { method: "DELETE", credentials: "same-origin" })
  })
})
