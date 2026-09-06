import { describe, expect, it } from "vitest"
import { routeOwner } from "./route-owner"

describe("route owner", () => {
  it("assigns the project dashboard URL to its route owner", () => {
    expect(routeOwner({ kind: "project", workspaceID: "wsp_a", projectID: "prj_a" })).toBe("project-dashboard")
  })

  it("assigns every Project Notes URL to its route owner", () => {
    expect(routeOwner({ kind: "project-notes", workspaceID: "wsp_a", projectID: "prj_a" })).toBe("project-notes")
    expect(routeOwner({ kind: "project-note-new", workspaceID: "wsp_a", projectID: "prj_a" })).toBe("project-notes")
    expect(routeOwner({ kind: "project-note", workspaceID: "wsp_a", projectID: "prj_a", noteID: "pnt_a" })).toBe("project-notes")
  })

  it("keeps non-Project Notes routes on the legacy outlet", () => {
    expect(routeOwner({ kind: "login", next: null })).toBe("legacy")
  })
})
