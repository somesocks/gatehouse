import { afterEach, expect, it, vi } from "vitest"
import { addProjectMember, fetchProjectMembers, updateProjectGrant } from "./project-members"

afterEach(() => vi.unstubAllGlobals())

it("uses encoded project paths, both subject kinds, and revision-checked updates", async () => {
  const request = vi.fn(async () => Response.json({}))
  vi.stubGlobal("fetch", request)
  const signal = new AbortController().signal
  await fetchProjectMembers("wsp/a", "prj/b", "principal", "Alice & Bob", "prn_cursor", signal)
  await fetchProjectMembers("wsp/a", "prj/b", "group", "", "", signal, true)
  await addProjectMember("wsp/a", "prj/b", "principal", "prn_a", "guest", signal)
  await addProjectMember("wsp/a", "prj/b", "group", "grp_a", "admin", signal)
  await updateProjectGrant("wsp/a", "prj/b", { id: "pgr/a", role: "guest", enabled: true, revision: 7 }, { enabled: false }, signal)
  expect(request.mock.calls).toEqual([
    ["/api/v1/workspaces/wsp%2Fa/projects/prj%2Fb/members?kind=principal&limit=50&name=Alice+%26+Bob&cursor=prn_cursor", expect.objectContaining({ signal, credentials: "same-origin" })],
    ["/api/v1/workspaces/wsp%2Fa/projects/prj%2Fb/group-options?kind=group&limit=50", expect.objectContaining({ signal })],
    ["/api/v1/workspaces/wsp%2Fa/projects/prj%2Fb/grants", expect.objectContaining({ method: "POST", body: JSON.stringify({ principal: "prn_a", role: "guest" }), signal })],
    ["/api/v1/workspaces/wsp%2Fa/projects/prj%2Fb/grants", expect.objectContaining({ method: "POST", body: JSON.stringify({ group: "grp_a", role: "admin" }), signal })],
    ["/api/v1/workspaces/wsp%2Fa/projects/prj%2Fb/grants/pgr%2Fa", expect.objectContaining({ method: "PATCH", body: JSON.stringify({ enabled: false, expected_revision: 7 }), signal })],
  ])
})
