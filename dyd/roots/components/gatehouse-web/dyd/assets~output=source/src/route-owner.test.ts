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

  it("assigns the Chat collection URL to its route owner", () => {
    expect(routeOwner({ kind: "session-collection", workspaceID: "wsp_a", search: "" })).toBe("chat-collection")
  })

  it("assigns the Project collection URL to its route owner", () => {
    expect(routeOwner({ kind: "project-collection", workspaceID: "wsp_a", search: "" })).toBe("project-collection")
  })

  it("assigns every System URL to its route owner", () => {
    expect(routeOwner({ kind: "system" })).toBe("system")
    expect(routeOwner({ kind: "system-grants" })).toBe("system")
    expect(routeOwner({ kind: "system-principals" })).toBe("system")
  })

  it("assigns workspace fallback URLs to the dashboard owner", () => {
    expect(routeOwner({ kind: "app-home" })).toBe("workspace-dashboard")
    expect(routeOwner({ kind: "workspace", workspaceID: "wsp_a" })).toBe("workspace-dashboard")
    expect(routeOwner({ kind: "not-found" })).toBe("workspace-dashboard")
  })

  it("assigns terminal access URLs to the access owner", () => {
    expect(routeOwner({ kind: "login", next: null })).toBe("access")
    expect(routeOwner({ kind: "no-access" })).toBe("access")
  })
})
