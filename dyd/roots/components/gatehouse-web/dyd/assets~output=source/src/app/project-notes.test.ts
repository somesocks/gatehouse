import { afterEach, describe, expect, it, vi } from "vitest"
import { createProjectNote, fetchProjectNote, removeProjectNote, updateProjectNote } from "./project-notes"

afterEach(() => vi.unstubAllGlobals())

describe("project notes transport", () => {
  it("uses encoded resource paths and same-origin credentials", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const input = { title: "A note", description: "Description", body: "Body", sensitive: true }
    await fetchProjectNote("wsp/test", "prj/test", "note/test")
    await createProjectNote("wsp/test", "prj/test", input)
    await updateProjectNote("wsp/test", "prj/test", "note/test", input)
    await removeProjectNote("wsp/test", "prj/test", "note/test")
    expect(fetch).toHaveBeenNthCalledWith(1, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/notes/note%2Ftest", { credentials: "same-origin" })
    expect(fetch).toHaveBeenNthCalledWith(2, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/notes", expect.objectContaining({ method: "POST", credentials: "same-origin" }))
    expect(fetch).toHaveBeenNthCalledWith(3, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/notes/note%2Ftest", expect.objectContaining({ method: "PATCH", credentials: "same-origin" }))
    expect(fetch).toHaveBeenNthCalledWith(4, "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/notes/note%2Ftest", { method: "DELETE", credentials: "same-origin" })
  })
})
