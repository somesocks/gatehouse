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
    {
      path: "/app/login?next=%2Fapp%2Fwsp%2Fwsp_a",
      route: { kind: "login", next: "/app/wsp/wsp_a" },
    },
    { path: "/app/no-access", route: { kind: "no-access" } },
    { path: "/app/input", route: { kind: "input-form" } },
    { path: "/app/system", route: { kind: "system" } },
    { path: "/app/system/grants", route: { kind: "system-grants" } },
    { path: "/app/system/principals", route: { kind: "system-principals" } },
    {
      path: "/app/system/agent-providers",
      route: { kind: "system-agent-providers" },
    },
    {
      path: "/app/system/agent-providers/new",
      route: { kind: "system-agent-provider-new" },
    },
    {
      path: "/app/system/agent-providers/apr_1",
      route: { kind: "system-agent-provider", providerID: "apr_1" },
    },
    {
      path: "/app/system/agent-models",
      route: { kind: "system-agent-models" },
    },
    {
      path: "/app/system/agent-models/new",
      route: { kind: "system-agent-model-new" },
    },
    {
      path: "/app/system/agent-models/amd_1",
      route: { kind: "system-agent-model", modelID: "amd_1" },
    },
    {
      path: "/app/system/storage-providers",
      route: { kind: "system-storage-providers" },
    },
    {
      path: "/app/system/storage-providers/new",
      route: { kind: "system-storage-provider-new" },
    },
    {
      path: "/app/system/storage-providers/stp_1",
      route: { kind: "system-storage-provider", providerID: "stp_1" },
    },
    {
      path: "/app/system/workspace-agent-bindings",
      route: { kind: "system-workspace-agent-bindings" },
    },
    {
      path: "/app/system/workspace-agent-bindings/new",
      route: { kind: "system-workspace-agent-binding-new" },
    },
    {
      path: "/app/system/workspace-agent-bindings/wsp_a/wag_1",
      route: {
        kind: "system-workspace-agent-binding",
        workspaceID: "wsp_a",
        bindingID: "wag_1",
      },
    },
    {
      path: "/app/system/workspace-storage-bindings",
      route: { kind: "system-workspace-storage-bindings" },
    },
    {
      path: "/app/system/workspace-storage-bindings/new",
      route: { kind: "system-workspace-storage-binding-new" },
    },
    {
      path: "/app/system/workspace-storage-bindings/wsp_a/stp_1",
      route: {
        kind: "system-workspace-storage-binding",
        workspaceID: "wsp_a",
        providerID: "stp_1",
      },
    },
    {
      path: "/app/wsp/wsp_a",
      route: { kind: "workspace", workspaceID: "wsp_a" },
    },
    {
      path: "/app/wsp/wsp_a/ses?name=planning",
      route: {
        kind: "session-collection",
        workspaceID: "wsp_a",
        search: "planning",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj?name=release",
      route: {
        kind: "project-collection",
        workspaceID: "wsp_a",
        search: "release",
      },
    },
    {
      path: "/app/wsp/wsp_a/grp",
      route: { kind: "group-collection", workspaceID: "wsp_a" },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a",
      route: {
        kind: "session-chat",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
        mode: "group",
        agent: null,
      },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a?mode=direct&agent=wag_a",
      route: {
        kind: "session-chat",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
        mode: "direct",
        agent: "wag_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a/files",
      route: {
        kind: "session-files",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a/notes",
      route: {
        kind: "session-notes",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a/notes/new",
      route: {
        kind: "session-note-new",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a/notes/snt_a",
      route: {
        kind: "session-note",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
        noteID: "snt_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a/notes/snt_a/edit",
      route: {
        kind: "session-note-edit",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
        noteID: "snt_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a/notes/snt_a/revisions/2",
      route: {
        kind: "session-note-revision",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
        noteID: "snt_a",
        revision: 2,
      },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a/tasks",
      route: {
        kind: "session-tasks",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a/tasks/new",
      route: {
        kind: "session-task-new",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a/tasks/stk_a",
      route: {
        kind: "session-task",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
        taskID: "stk_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a/secrets",
      route: {
        kind: "session-secrets",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a/secrets/new",
      route: {
        kind: "session-secret-new",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a/secrets/ssr_a",
      route: {
        kind: "session-secret",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
        secretID: "ssr_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a",
      route: { kind: "project", workspaceID: "wsp_a", projectID: "prj_a" },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a/pnt",
      route: {
        kind: "project-notes",
        workspaceID: "wsp_a",
        projectID: "prj_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a/pnt/new",
      route: {
        kind: "project-note-new",
        workspaceID: "wsp_a",
        projectID: "prj_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a/pnt/pnt_a",
      route: {
        kind: "project-note",
        workspaceID: "wsp_a",
        projectID: "prj_a",
        noteID: "pnt_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a/tasks",
      route: {
        kind: "project-tasks",
        workspaceID: "wsp_a",
        projectID: "prj_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a/tasks/new",
      route: {
        kind: "project-task-new",
        workspaceID: "wsp_a",
        projectID: "prj_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a/tasks/ptk_a",
      route: {
        kind: "project-task",
        workspaceID: "wsp_a",
        projectID: "prj_a",
        taskID: "ptk_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a/records",
      route: {
        kind: "project-records",
        workspaceID: "wsp_a",
        projectID: "prj_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a/records/sch_a/rec_a",
      route: {
        kind: "project-record",
        workspaceID: "wsp_a",
        projectID: "prj_a",
        schemaID: "sch_a",
        recordID: "rec_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a/records/sch_a/rec_a/edit",
      route: {
        kind: "project-record-edit",
        workspaceID: "wsp_a",
        projectID: "prj_a",
        schemaID: "sch_a",
        recordID: "rec_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a/files",
      route: {
        kind: "project-files",
        workspaceID: "wsp_a",
        projectID: "prj_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a/files/pfl_a",
      route: {
        kind: "project-file",
        workspaceID: "wsp_a",
        projectID: "prj_a",
        fileID: "pfl_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a/secrets",
      route: {
        kind: "project-secrets",
        workspaceID: "wsp_a",
        projectID: "prj_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a/secrets/new",
      route: {
        kind: "project-secret-new",
        workspaceID: "wsp_a",
        projectID: "prj_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/prj/prj_a/secrets/psr_a",
      route: {
        kind: "project-secret",
        workspaceID: "wsp_a",
        projectID: "prj_a",
        secretID: "psr_a",
      },
    },
    {
      path: "/app/wsp/wsp_a/ses/ses_a/notes/",
      route: {
        kind: "session-notes",
        workspaceID: "wsp_a",
        sessionID: "ses_a",
      },
      canonical: "/app/wsp/wsp_a/ses/ses_a/notes",
    },
    {
      path: "/app/wsp/work%20space/grp",
      route: { kind: "group-collection", workspaceID: "work space" },
      canonical: "/app/wsp/work%20space/grp",
    },
  ]

  for (const entry of cases) {
    it(`parses ${entry.path}`, () => {
      expect(parseRoute(url(entry.path))).toEqual(entry.route)
      expect(routePath(entry.route)).toBe(entry.canonical ?? entry.path)
    })
  }

  for (const path of [
    "/",
    "/app/wsp",
    "/app/wsp/wsp_a/ses/ses_a/notes/new/edit",
    "/app/wsp/wsp_a/ses/ses_a/notes/snt_a/revisions/0",
    "/app/wsp/wsp_a/ses/ses_a/notes/snt_a/revisions/1.5",
    "/app/wsp/%ZZ",
    "/app/wsp//ses",
  ]) {
    it(`rejects ${path}`, () => {
      expect(parseRoute(url(path))).toEqual({ kind: "not-found" })
    })
  }

  it("identifies session routes", () => {
    expect(
      parseRoute(url("/app/wsp/wsp_a/ses/ses_a/notes/snt_a")),
    ).toMatchObject({ sessionID: "ses_a" })
    expect(parseRoute(url("/app/wsp/wsp_a/prj/prj_a"))).not.toHaveProperty(
      "sessionID",
    )
  })
})
