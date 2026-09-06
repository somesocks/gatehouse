import { describe, expect, it } from "vitest"
import { acceptsProjectNotesResult, activateProjectNotesRoute, settledProjectNotesListStatus } from "./lifecycle"

describe("Project Notes lifecycle", () => {
  it("resets and settles the list after creating, viewing a detail, then returning", () => {
    const creating = activateProjectNotesRoute("project-note-new")
    expect(creating).toMatchObject({ creating: true, editing: true, notesStatus: "checking", detailStatus: "ready" })

    const detail = activateProjectNotesRoute("project-note")
    expect(detail).toMatchObject({ creating: false, editing: false, notesStatus: "checking", detailStatus: "checking" })

    const list = activateProjectNotesRoute("project-notes")
    expect(list).toMatchObject({ creating: false, editing: false, notesStatus: "checking", detailStatus: "ready" })
    expect({ ...list, notesStatus: settledProjectNotesListStatus(true) }).toMatchObject({ notesStatus: "ready" })
    expect({ ...list, notesStatus: settledProjectNotesListStatus(false) }).toMatchObject({ notesStatus: "unavailable" })
  })

  it("rejects stale workspace, project, and generation results", () => {
    expect(acceptsProjectNotesResult(4, 4, "wsp_a", "wsp_a", "prj_a", "prj_a")).toBe(true)
    expect(acceptsProjectNotesResult(4, 3, "wsp_a", "wsp_a", "prj_a", "prj_a")).toBe(false)
    expect(acceptsProjectNotesResult(4, 4, "wsp_a", "wsp_b", "prj_a", "prj_a")).toBe(false)
    expect(acceptsProjectNotesResult(4, 4, "wsp_a", "wsp_a", "prj_a", "prj_b")).toBe(false)
  })
})
