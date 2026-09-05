import { afterEach, describe, expect, it, vi } from "vitest"
import { fetchProjects } from "./projects"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("projects transport", () => {
  it("searches projects with session credentials", async () => {
    const fetch = vi.fn(async () => Response.json({ projects: [] }))
    vi.stubGlobal("fetch", fetch)

    await fetchProjects("wsp/test", "roadmap", "prj_1")

    expect(fetch).toHaveBeenCalledWith("/api/v1/workspaces/wsp%2Ftest/projects?limit=50&name=roadmap&cursor=prj_1", { credentials: "same-origin" })
  })
})
