import { describe, expect, it } from "vitest"
import { parseRoute, routePath, type NavigableRoute } from "./route"
import { routeOwner } from "./route-owner"

describe("workspace page-purpose routes", () => {
  it.each([
    ["/app/wsp/wsp_a/grp/grp_a", "group", "groups"],
    ["/app/wsp/wsp_a/prj/new", "project-new", "project-form"],
    ["/app/wsp/wsp_a/prj/prj_a/edit", "project-edit", "project-form"],
    ["/app/wsp/wsp_a/prj/prj_a/members", "project-members", "project-members"],
    [
      "/app/wsp/wsp_a/prj/prj_a/pnt/pnt_a/edit",
      "project-note-edit",
      "project-notes",
    ],
    [
      "/app/wsp/wsp_a/prj/prj_a/pnt/pnt_a/revisions/2",
      "project-note-revision",
      "project-notes",
    ],
    [
      "/app/wsp/wsp_a/prj/prj_a/tasks/ptk_a/edit",
      "project-task-edit",
      "project-tasks",
    ],
    [
      "/app/wsp/wsp_a/ses/ses_a/tasks/stk_a/edit",
      "session-task-edit",
      "session-tasks",
    ],
    [
      "/app/wsp/wsp_a/ses/ses_a/secrets/ssc_a/edit",
      "session-secret-edit",
      "session-secrets",
    ],
    [
      "/app/wsp/wsp_a/prj/prj_a/records/prs_a/overview",
      "project-record-schema-overview",
      "project-records",
    ],
    [
      "/app/wsp/wsp_a/prj/prj_a/records/prs_a/fields",
      "project-record-fields",
      "project-records",
    ],
    [
      "/app/wsp/wsp_a/prj/prj_a/records/prs_a/fields/new",
      "project-record-field-new",
      "project-records",
    ],
    [
      "/app/wsp/wsp_a/prj/prj_a/records/prs_a/fields/pra_a/edit",
      "project-record-field-edit",
      "project-records",
    ],
  ])("round-trips %s and selects its resource owner", (path, kind, owner) => {
    const route = parseRoute(new URL(path, "http://localhost"))
    expect(route.kind).toBe(kind)
    expect(routeOwner(route)).toBe(owner)
    expect(routePath(route as NavigableRoute)).toBe(path)
  })
  it("retains project scope and name search in a chat directory URL", () => {
    const route = parseRoute(
      new URL("http://localhost/app/wsp/wsp_a/ses?name=review&project=prj_a"),
    )
    expect(route).toEqual({
      kind: "session-collection",
      workspaceID: "wsp_a",
      search: "review",
      projectID: "prj_a",
    })
    expect(routePath(route as NavigableRoute)).toBe(
      "/app/wsp/wsp_a/ses?name=review&project=prj_a",
    )
  })
  it.each([
    "/app/wsp/wsp_a/prj/prj_a/pnt/new/edit",
    "/app/wsp/wsp_a/prj/prj_a/pnt/pnt_a/revisions/0",
    "/app/wsp/wsp_a/prj/prj_a/records/prs_a/fields/new/edit",
    "/app/wsp/wsp_a/prj/prj_a/members/new",
    "/app/wsp/wsp_a/prj/prj_a/members/person/edit",
  ])("rejects malformed page-purpose path %s", (path) => {
    expect(parseRoute(new URL(path, "http://localhost"))).toEqual({
      kind: "not-found",
    })
  })
})
