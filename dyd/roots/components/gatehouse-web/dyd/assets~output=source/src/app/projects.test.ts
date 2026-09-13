import { afterEach, describe, expect, it, vi } from "vitest"
import {
  createProject,
  fetchDashboardProjects,
  fetchProjects,
} from "./projects"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("projects transport", () => {
  it("searches projects with session credentials and route cancellation", async () => {
    const fetch = vi.fn(async () => Response.json({ projects: [] }))
    vi.stubGlobal("fetch", fetch)
    const controller = new AbortController()

    await fetchProjects("wsp/test", "roadmap", "prj_1", controller.signal)

    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/workspaces/wsp%2Ftest/projects?limit=50&name=roadmap&cursor=prj_1",
      { credentials: "same-origin", signal: controller.signal },
    )
  })

  it("creates workspace projects", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const controller = new AbortController()

    await createProject("wsp/test", controller.signal)

    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/workspaces/wsp%2Ftest/projects",
      {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({}),
        signal: controller.signal,
      },
    )
  })

  it("loads the five latest projects for a dashboard", async () => {
    const fetch = vi.fn(async () => Response.json({ projects: [] }))
    vi.stubGlobal("fetch", fetch)
    const controller = new AbortController()

    await fetchDashboardProjects("wsp/test", controller.signal)

    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/workspaces/wsp%2Ftest/projects?limit=5",
      { credentials: "same-origin", signal: controller.signal },
    )
  })
})
