import { afterEach, expect, it, vi } from "vitest"
import { createGroupSecret, fetchGroupSecrets, removeGroupSecret, updateGroupSecret } from "./group-secrets"

afterEach(() => vi.unstubAllGlobals())
it("encodes group paths and checks revisions without round-tripping credential values", async () => {
  const request = vi.fn(async () => Response.json({}))
  vi.stubGlobal("fetch", request)
  const signal = new AbortController().signal
  const secret = { id: "gsc/a", group: "group", description: "Token", author: { id: "owner" }, revision: 7, created_at: "now", updated_at: "now" }
  await fetchGroupSecrets("wsp/a", "grp/b", "Deploy & test", "gsc_cursor", signal)
  await createGroupSecret("wsp/a", "grp/b", "Token", "new value", signal)
  await updateGroupSecret("wsp/a", "grp/b", secret, "Changed", "", signal)
  await removeGroupSecret("wsp/a", "grp/b", secret, signal)
  expect(request.mock.calls).toEqual([
    ["/api/v1/workspaces/wsp%2Fa/groups/grp%2Fb/secrets?limit=50&name=Deploy+%26+test&cursor=gsc_cursor", expect.objectContaining({ credentials: "same-origin", signal })],
    ["/api/v1/workspaces/wsp%2Fa/groups/grp%2Fb/secrets", expect.objectContaining({ method: "POST", body: JSON.stringify({ description: "Token", value: "new value" }), signal })],
    ["/api/v1/workspaces/wsp%2Fa/groups/grp%2Fb/secrets/gsc%2Fa", expect.objectContaining({ method: "PATCH", body: JSON.stringify({ description: "Changed", expected_revision: 7 }), signal })],
    ["/api/v1/workspaces/wsp%2Fa/groups/grp%2Fb/secrets/gsc%2Fa", expect.objectContaining({ method: "DELETE", body: JSON.stringify({ expected_revision: 7 }), signal })],
  ])
})
