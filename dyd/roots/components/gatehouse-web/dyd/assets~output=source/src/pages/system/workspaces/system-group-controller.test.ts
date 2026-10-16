import { afterEach, describe, expect, it, vi } from "vitest"
import { clearAccessToken } from "../../../app/api"
import {
  type SystemGroup,
  type SystemGroupGrant,
} from "../../../app/group-administration"
import { createSystemGroupController } from "./system-group-controller.svelte"
import { parseRoute, routePath } from "../../../route"
import { routeOwner } from "../../../route-owner"

afterEach(() => {
  clearAccessToken()
  vi.unstubAllGlobals()
})
const group: SystemGroup = {
  id: "grp_developers",
  workspace: "wsp_team",
  alias: "developers",
  name: "Developers",
  enabled: true,
  revision: 1,
}
const grant: SystemGroupGrant = {
  id: "wgr_team",
  workspace: group.workspace,
  group: group.id,
  role: "contributor",
  enabled: true,
  revision: 3,
}
function fixture() {
  const onAuthenticationLost = vi.fn(),
    onSystemAccessChange = vi.fn(),
    onMembersChanged = vi.fn(async () => true)
  const controller = createSystemGroupController({
    workspaceID: () => group.workspace,
    groupID: () => group.id,
    onAuthenticationLost,
    onSystemAccessChange,
    onMembersChanged,
  })
  return {
    controller,
    onAuthenticationLost,
    onSystemAccessChange,
    onMembersChanged,
  }
}
function transport() {
  return vi.fn<typeof globalThis.fetch>(async (input, init) => {
    if (!init?.method || init.method === "GET")
      return String(input).endsWith("/grants")
        ? Response.json({ grants: [grant] })
        : Response.json(group)
    return Response.json(group)
  })
}

describe("system group administration", () => {
  it("routes group lists and details under system workspace administration", () => {
    for (const path of [
      "/app/system/workspaces/wsp_team/groups",
      "/app/system/workspaces/wsp_team/groups/grp_developers",
    ]) {
      const route = parseRoute(new URL("https://gatehouse.test" + path))
      expect(routeOwner(route)).toBe("system-workspace-groups")
      if (route.kind === "not-found") throw new Error("group route not found")
      expect(routePath(route)).toBe(path)
    }
  })
  it("creates a group using the alias, name and enabled state", async () => {
    const fetch = transport()
    vi.stubGlobal("fetch", fetch)
    const { controller } = fixture()
    Object.assign(controller.state.form, {
      alias: group.alias,
      name: group.name,
      enabled: true,
    })
    expect(await controller.create()).toEqual(group)
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/system/workspaces/wsp_team/groups",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          alias: group.alias,
          name: group.name,
          enabled: true,
        }),
      }),
    )
    controller.stop()
  })
  it("updates group settings with the loaded revision", async () => {
    const fetch = transport()
    vi.stubGlobal("fetch", fetch)
    const { controller } = fixture()
    await controller.load()
    controller.state.form.name = "Renamed"
    controller.state.form.enabled = false
    await controller.saveGroup()
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/system/workspaces/wsp_team/groups/grp_developers",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({
          alias: "developers",
          name: "Renamed",
          enabled: false,
          expected_revision: 1,
        }),
      }),
    )
    controller.stop()
  })
  it("preserves metadata drafts and their revision while refreshing grants", async () => {
    const fetch = transport()
    vi.stubGlobal("fetch", fetch)
    const { controller } = fixture()
    await controller.load()
    controller.state.form.name = "Unsaved draft"
    await controller.toggleGrant(grant)
    expect(controller.state.form.name).toBe("Unsaved draft")
    expect(controller.state.form.revision).toBe(1)
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/system/workspaces/wsp_team/grants/wgr_team",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({ enabled: false, expected_revision: 3 }),
      }),
    )
    controller.stop()
  })
  it("requires replacing stale group drafts instead of silently retrying", async () => {
    const fetch = transport()
    vi.stubGlobal("fetch", fetch)
    const { controller } = fixture()
    await controller.load()
    fetch.mockResolvedValueOnce(
      new Response("This record changed elsewhere. Reload before retrying.", {
        status: 409,
      }),
    )
    controller.state.form.name = "Draft"
    await controller.saveGroup()
    expect(controller.state.groupConflict).toBe(true)
    expect(controller.state.form.name).toBe("Draft")
    const count = fetch.mock.calls.length
    await controller.saveGroup()
    expect(fetch).toHaveBeenCalledTimes(count)
    await controller.load(true)
    expect(controller.state.groupConflict).toBe(false)
    expect(controller.state.form.name).toBe("Developers")
    controller.stop()
  })
  it("adds and disables individual memberships without changing group settings", async () => {
    const fetch = transport()
    vi.stubGlobal("fetch", fetch)
    const { controller, onMembersChanged } = fixture()
    controller.state.principal = "prn_alice"
    await controller.addMember()
    expect(controller.state.principal).toBe("")
    await controller.toggleMember({
      principal: "prn_alice",
      principal_enabled: true,
      enabled: true,
      revision: 2,
    })
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/system/workspaces/wsp_team/groups/grp_developers/members/prn_alice",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({ enabled: false, expected_revision: 2 }),
      }),
    )
    expect(onMembersChanged).toHaveBeenCalledTimes(2)
    controller.stop()
  })
  it("edits the selected grant's role and enabled status using its revision", async () => {
    const fetch = transport()
    vi.stubGlobal("fetch", fetch)
    const { controller } = fixture()
    await controller.load()
    controller.beginGrant(grant)
    controller.state.grantRole = "manager"
    controller.state.grantEnabled = false
    await controller.saveGrant()
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/system/workspaces/wsp_team/grants/wgr_team",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({
          role: "manager",
          enabled: false,
          expected_revision: 3,
        }),
      }),
    )
    expect(controller.state.editingGrant).toBeNull()
    controller.stop()
  })
  it("initializes the form when retrying a failed initial load", async () => {
    const fetch = transport().mockRejectedValueOnce(new Error("offline"))
    vi.stubGlobal("fetch", fetch)
    const { controller } = fixture()
    await controller.load()
    expect(controller.state.group).toBeNull()
    await controller.load(false)
    expect(controller.state.form.alias).toBe("developers")
    expect(controller.state.form.revision).toBe(1)
    controller.stop()
  })
  it.each([401, 403])(
    "handles loss of sysadmin access (%s)",
    async (status) => {
      vi.stubGlobal(
        "fetch",
        vi.fn(async () => new Response(null, { status })),
      )
      const { controller, onAuthenticationLost, onSystemAccessChange } =
        fixture()
      await controller.load()
      if (status === 401) expect(onAuthenticationLost).toHaveBeenCalledOnce()
      else expect(onSystemAccessChange).toHaveBeenCalledWith("denied")
      controller.stop()
    },
  )
  it("aborts requests and ignores responses after leaving a group page", async () => {
    let resolve!: (value: Response) => void
    const fetch = vi.fn<typeof globalThis.fetch>(
      () =>
        new Promise<Response>((done) => {
          resolve = done
        }),
    )
    vi.stubGlobal("fetch", fetch)
    const { controller } = fixture()
    const pending = controller.create()
    controller.stop()
    expect(fetch.mock.calls[0][1]?.signal?.aborted).toBe(true)
    resolve(Response.json(group))
    expect(await pending).toBeNull()
    expect(controller.state.notice).toBe("")
  })
})
