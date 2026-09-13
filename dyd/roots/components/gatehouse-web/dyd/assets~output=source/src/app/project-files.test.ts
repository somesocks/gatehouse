import { afterEach, describe, expect, it, vi } from "vitest"
import {
  finishProjectFileUpload,
  projectFileDownloadPath,
  removeProjectFile,
  startProjectFileUpload,
} from "./project-files"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("project files transport", () => {
  it("uses encoded project paths for upload lifecycle and downloads", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const file = new File(["body"], "plan.txt", { type: "text/plain" })

    await startProjectFileUpload("wsp/test", "prj/test", file)
    await finishProjectFileUpload("wsp/test", "prj/test", "fil/test")
    await removeProjectFile("wsp/test", "prj/test", "fil/test")

    expect(fetch).toHaveBeenNthCalledWith(
      1,
      "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/files/start",
      expect.objectContaining({
        method: "POST",
        credentials: "same-origin",
        body: JSON.stringify({ name: "plan.txt", media_type: "text/plain" }),
      }),
    )
    expect(fetch).toHaveBeenNthCalledWith(
      2,
      "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/files/fil%2Ftest/finish",
      { method: "POST", credentials: "same-origin", signal: undefined },
    )
    expect(fetch).toHaveBeenNthCalledWith(
      3,
      "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/files/fil%2Ftest",
      { method: "DELETE", credentials: "same-origin", signal: undefined },
    )
    expect(projectFileDownloadPath("wsp/test", "prj/test", "fil/test")).toBe(
      "/api/v1/workspaces/wsp%2Ftest/projects/prj%2Ftest/files/fil%2Ftest/download",
    )
  })
})
