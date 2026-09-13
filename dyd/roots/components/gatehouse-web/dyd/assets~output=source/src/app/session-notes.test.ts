import { afterEach, describe, expect, it, vi } from "vitest"
import {
  createSessionNote,
  fetchSessionNote,
  fetchSessionNoteRevision,
  fetchSessionNoteRevisions,
  fetchSessionNotes,
  removeSessionNote,
  updateSessionNote,
} from "./session-notes"

afterEach(() => vi.unstubAllGlobals())

describe("session notes transport", () => {
  it("uses encoded resource paths, same-origin credentials, and route cancellation", async () => {
    const fetch = vi.fn(async () => Response.json({}))
    vi.stubGlobal("fetch", fetch)
    const controller = new AbortController()
    const input = {
      title: "A note",
      description: "Description",
      body: "Body",
      sensitive: true,
    }
    await fetchSessionNotes("wsp/test", "ses/test", controller.signal)
    await fetchSessionNote(
      "wsp/test",
      "ses/test",
      "note/test",
      controller.signal,
    )
    await fetchSessionNoteRevisions(
      "wsp/test",
      "ses/test",
      "note/test",
      controller.signal,
    )
    await fetchSessionNoteRevision(
      "wsp/test",
      "ses/test",
      "note/test",
      2,
      controller.signal,
    )
    await createSessionNote("wsp/test", "ses/test", input, controller.signal)
    await updateSessionNote(
      "wsp/test",
      "ses/test",
      "note/test",
      input,
      controller.signal,
    )
    await removeSessionNote(
      "wsp/test",
      "ses/test",
      "note/test",
      controller.signal,
    )
    expect(fetch).toHaveBeenNthCalledWith(
      1,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/notes",
      { credentials: "same-origin", signal: controller.signal },
    )
    expect(fetch).toHaveBeenNthCalledWith(
      2,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/notes/note%2Ftest",
      { credentials: "same-origin", signal: controller.signal },
    )
    expect(fetch).toHaveBeenNthCalledWith(
      3,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/notes/note%2Ftest/revisions",
      { credentials: "same-origin", signal: controller.signal },
    )
    expect(fetch).toHaveBeenNthCalledWith(
      4,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/notes/note%2Ftest/revisions/2",
      { credentials: "same-origin", signal: controller.signal },
    )
    expect(fetch).toHaveBeenNthCalledWith(
      5,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/notes",
      expect.objectContaining({
        method: "POST",
        credentials: "same-origin",
        signal: controller.signal,
      }),
    )
    expect(fetch).toHaveBeenNthCalledWith(
      6,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/notes/note%2Ftest",
      expect.objectContaining({
        method: "PATCH",
        credentials: "same-origin",
        signal: controller.signal,
      }),
    )
    expect(fetch).toHaveBeenNthCalledWith(
      7,
      "/api/v1/workspaces/wsp%2Ftest/sessions/ses%2Ftest/notes/note%2Ftest",
      {
        method: "DELETE",
        credentials: "same-origin",
        signal: controller.signal,
      },
    )
  })
})
