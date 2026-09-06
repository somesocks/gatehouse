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

  it("assigns every Project Secrets URL to its route owner", () => {
    expect(routeOwner({ kind: "project-secrets", workspaceID: "wsp_a", projectID: "prj_a" })).toBe("project-secrets")
    expect(routeOwner({ kind: "project-secret-new", workspaceID: "wsp_a", projectID: "prj_a" })).toBe("project-secrets")
    expect(routeOwner({ kind: "project-secret", workspaceID: "wsp_a", projectID: "prj_a", secretID: "sec_a" })).toBe("project-secrets")
  })

  it("assigns session detail URLs to their route owners", () => {
    expect(routeOwner({ kind: "session-chat", workspaceID: "wsp_a", sessionID: "ses_a" })).toBe("session-chat")
    expect(routeOwner({ kind: "session-notes", workspaceID: "wsp_a", sessionID: "ses_a" })).toBe("session-notes")
    expect(routeOwner({ kind: "session-note-new", workspaceID: "wsp_a", sessionID: "ses_a" })).toBe("session-notes")
    expect(routeOwner({ kind: "session-note", workspaceID: "wsp_a", sessionID: "ses_a", noteID: "snt_a" })).toBe("session-notes")
    expect(routeOwner({ kind: "session-note-edit", workspaceID: "wsp_a", sessionID: "ses_a", noteID: "snt_a" })).toBe("session-notes")
    expect(routeOwner({ kind: "session-note-revision", workspaceID: "wsp_a", sessionID: "ses_a", noteID: "snt_a", revision: 2 })).toBe("session-notes")
    expect(routeOwner({ kind: "session-secrets", workspaceID: "wsp_a", sessionID: "ses_a" })).toBe("session-secrets")
    expect(routeOwner({ kind: "session-secret-new", workspaceID: "wsp_a", sessionID: "ses_a" })).toBe("session-secrets")
    expect(routeOwner({ kind: "session-secret", workspaceID: "wsp_a", sessionID: "ses_a", secretID: "sec_a" })).toBe("session-secrets")
  })

  it("assigns the Groups collection URL to its route owner", () => {
    expect(routeOwner({ kind: "group-collection", workspaceID: "wsp_a" })).toBe("groups")
  })

  it("keeps non-Project Notes routes on the legacy outlet", () => {
    expect(routeOwner({ kind: "login", next: null })).toBe("legacy")
  })
})
