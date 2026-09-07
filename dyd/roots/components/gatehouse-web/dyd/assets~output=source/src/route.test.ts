import { describe, expect, it } from "vitest"
import { parseRoute, routePath } from "./route"
import type { NavigableRoute } from "./route"

const origin = "https://gatehouse.test"

function url(path: string): URL {
  return new URL(path, origin)
}

describe("routes", () => {
  const cases: { path: string; route: NavigableRoute; canonical?: string }[] = [
    { path: "/app/", route: { kind: "app-home" } },
    { path: "/app/login?next=%2Fapp%2Fwsp%2Fwsp_a", route: { kind: "login", next: "/app/wsp/wsp_a" } },
    { path: "/app/no-access", route: { kind: "no-access" } },
    { path: "/app/system", route: { kind: "system" } },
    { path: "/app/system/grants", route: { kind: "system-grants" } },
    { path: "/app/system/principals", route: { kind: "system-principals" } },
    { path: "/app/system/agent-providers", route: { kind: "system-agent-providers" } },
    { path: "/app/system/agent-providers/new", route: { kind: "system-agent-provider-new" } },
    { path: "/app/system/agent-providers/apr_1", route: { kind: "system-agent-provider", providerID: "apr_1" } },
    { path: "/app/system/agent-models", route: { kind: "system-agent-models" } },
    { path: "/app/system/storage-providers", route: { kind: "system-storage-providers" } },
    { path: "/app/system/workspace-agent-bindings", route: { kind: "system-workspace-agent-bindings" } },
    { path: "/app/system/workspace-storage-bindings", route: { kind: "system-workspace-storage-bindings" } },
    { path: "/app/wsp/wsp_a", route: { kind: "workspace", workspaceID: "wsp_a" } },
    { path: "/app/wsp/wsp_a/ses?name=planning", route: { kind: "session-collection", workspaceID: "wsp_a", search: "planning" } },
    { path: "/app/wsp/wsp_a/prj?name=release", route: { kind: "project-collection", workspaceID: "wsp_a", search: "release" } },
    { path: "/app/wsp/wsp_a/grp", route: { kind: "group-collection", workspaceID: "wsp_a" } },
    { path: "/app/wsp/wsp_a/ses/ses_a", route: { kind: "session-chat", workspaceID: "wsp_a", sessionID: "ses_a" } },
    { path: "/app/wsp/wsp_a/ses/ses_a/notes", route: { kind: "session-notes", workspaceID: "wsp_a", sessionID: "ses_a" } },
    { path: "/app/wsp/wsp_a/ses/ses_a/notes/new", route: { kind: "session-note-new", workspaceID: "wsp_a", sessionID: "ses_a" } },
    { path: "/app/wsp/wsp_a/ses/ses_a/notes/snt_a", route: { kind: "session-note", workspaceID: "wsp_a", sessionID: "ses_a", noteID: "snt_a" } },
    { path: "/app/wsp/wsp_a/ses/ses_a/notes/snt_a/edit", route: { kind: "session-note-edit", workspaceID: "wsp_a", sessionID: "ses_a", noteID: "snt_a" } },
    { path: "/app/wsp/wsp_a/ses/ses_a/notes/snt_a/revisions/2", route: { kind: "session-note-revision", workspaceID: "wsp_a", sessionID: "ses_a", noteID: "snt_a", revision: 2 } },
    { path: "/app/wsp/wsp_a/ses/ses_a/secrets", route: { kind: "session-secrets", workspaceID: "wsp_a", sessionID: "ses_a" } },
    { path: "/app/wsp/wsp_a/ses/ses_a/secrets/new", route: { kind: "session-secret-new", workspaceID: "wsp_a", sessionID: "ses_a" } },
    { path: "/app/wsp/wsp_a/ses/ses_a/secrets/ssr_a", route: { kind: "session-secret", workspaceID: "wsp_a", sessionID: "ses_a", secretID: "ssr_a" } },
    { path: "/app/wsp/wsp_a/prj/prj_a", route: { kind: "project", workspaceID: "wsp_a", projectID: "prj_a" } },
    { path: "/app/wsp/wsp_a/prj/prj_a/pnt", route: { kind: "project-notes", workspaceID: "wsp_a", projectID: "prj_a" } },
    { path: "/app/wsp/wsp_a/prj/prj_a/pnt/new", route: { kind: "project-note-new", workspaceID: "wsp_a", projectID: "prj_a" } },
    { path: "/app/wsp/wsp_a/prj/prj_a/pnt/pnt_a", route: { kind: "project-note", workspaceID: "wsp_a", projectID: "prj_a", noteID: "pnt_a" } },
    { path: "/app/wsp/wsp_a/prj/prj_a/secrets", route: { kind: "project-secrets", workspaceID: "wsp_a", projectID: "prj_a" } },
    { path: "/app/wsp/wsp_a/prj/prj_a/secrets/new", route: { kind: "project-secret-new", workspaceID: "wsp_a", projectID: "prj_a" } },
    { path: "/app/wsp/wsp_a/prj/prj_a/secrets/psr_a", route: { kind: "project-secret", workspaceID: "wsp_a", projectID: "prj_a", secretID: "psr_a" } },
    { path: "/app/wsp/wsp_a/ses/ses_a/notes/", route: { kind: "session-notes", workspaceID: "wsp_a", sessionID: "ses_a" }, canonical: "/app/wsp/wsp_a/ses/ses_a/notes" },
    { path: "/app/wsp/work%20space/grp", route: { kind: "group-collection", workspaceID: "work space" }, canonical: "/app/wsp/work%20space/grp" },
  ]

  for (const entry of cases) {
    it(`parses ${entry.path}`, () => {
      expect(parseRoute(url(entry.path))).toEqual(entry.route)
      expect(routePath(entry.route)).toBe(entry.canonical ?? entry.path)
    })
  }

  for (const path of ["/", "/app/wsp", "/app/wsp/wsp_a/ses/ses_a/notes/new/edit", "/app/wsp/wsp_a/ses/ses_a/notes/snt_a/revisions/0", "/app/wsp/wsp_a/ses/ses_a/notes/snt_a/revisions/1.5", "/app/wsp/%ZZ", "/app/wsp//ses"]) {
    it(`rejects ${path}`, () => {
      expect(parseRoute(url(path))).toEqual({ kind: "not-found" })
    })
  }

  it("identifies session routes", () => {
    expect(parseRoute(url("/app/wsp/wsp_a/ses/ses_a/notes/snt_a"))).toMatchObject({ sessionID: "ses_a" })
    expect(parseRoute(url("/app/wsp/wsp_a/prj/prj_a"))).not.toHaveProperty("sessionID")
  })
})
