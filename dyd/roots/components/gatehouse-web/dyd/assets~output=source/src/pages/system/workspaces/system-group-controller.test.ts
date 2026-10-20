import { afterEach, describe, expect, it, vi } from "vitest"
import { clearAccessToken } from "../../../app/api"
import { type SystemGroup } from "../../../app/group-administration"
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
function fixture() {
  const onAuthenticationLost = vi.fn(),
    onSystemAccessChange = vi.fn(),
    onMembersChanged = vi.fn(async () => true),
    onAgentsChanged = vi.fn(async () => true)
  const controller = createSystemGroupController({
    workspaceID: () => group.workspace,
    groupID: () => group.id,
    onAuthenticationLost,
    onSystemAccessChange,
    onMembersChanged,
    onAgentsChanged,
  })
  return {
    controller,
    onAuthenticationLost,
    onSystemAccessChange,
    onMembersChanged,
    onAgentsChanged,
  }
}
function transport() {
  return vi.fn<typeof globalThis.fetch>(async (input, init) => {
    return Response.json(group)
  })
}

describe("system group administration", () => {
  it("opens a fresh details draft and only marks metadata edits dirty", async () => {
    vi.stubGlobal("fetch", transport())
    const { controller } = fixture()
    await controller.load()
    expect(controller.state.dirty).toBe(false)
    controller.state.form.name = "Cancelled draft"
    expect(controller.state.dirty).toBe(true)
    controller.beginDetails()
    expect(controller.state.form.name).toBe(group.name)
    expect(controller.state.form.revision).toBe(group.revision)
    expect(controller.state.dirty).toBe(false)
    controller.state.agent = "wag_assistant"
    expect(controller.state.dirty).toBe(false)
    controller.stop()
  })

  it("keeps bot-add progress and success scoped to the bot section", async () => {
    const fetch = transport()
    vi.stubGlobal("fetch", fetch)
    const { controller } = fixture()
    await controller.load()
    controller.state.feedback.members.notice = "Member added."
    controller.state.agent = "wag_assistant"
    let finish!: (response: Response) => void
    fetch.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const pending = controller.addAgent()
    expect(controller.state.saving).toBe(true)
    expect(controller.state.pendingAction).toBe("bots:add")
    expect(controller.state.feedback.details).toEqual({ error: "", notice: "" })
    finish(new Response(null, { status: 201 }))
    expect(await pending).toBe(true)
    expect(controller.state.pendingAction).toBe("")
    expect(controller.state.feedback.bots.notice).toBe("Bot added.")
    expect(controller.state.feedback.members.notice).toBe("Member added.")
    expect(controller.state.feedback.details.notice).toBe("")
    controller.stop()
  })

  it("keeps failed member additions in their dialog without changing other feedback", async () => {
    vi.stubGlobal(
      "fetch",
      transport().mockResolvedValueOnce(
        new Response("Membership already exists.", { status: 409 }),
      ),
    )
    const { controller } = fixture()
    controller.state.principal = "prn_alice"
    expect(await controller.addMember()).toBe(false)
    expect(controller.state.principal).toBe("prn_alice")
    expect(controller.state.feedback.members.error).toContain("already exists")
    expect(controller.state.feedback.bots.error).toBe("")
    expect(controller.state.feedback.details.error).toBe("")
    controller.stop()
  })

  it("loads group details without requesting group workspace grants", async () => {
    const fetch = transport()
    vi.stubGlobal("fetch", fetch)
    const { controller } = fixture()
    await controller.load()
    expect(fetch.mock.calls.map(([path]) => String(path))).toEqual([
      "/api/v1/system/workspaces/wsp_team/groups/grp_developers",
    ])
    controller.stop()
  })

  it("aborts a details refresh and ignores a late decoded result after disposal", async () => {
    const fetch = transport()
    vi.stubGlobal("fetch", fetch)
    const { controller } = fixture()
    await controller.load()
    let decode!: (body: object) => void
    fetch.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: () =>
        new Promise((resolve) => {
          decode = resolve
        }),
    } as Response)
    const pending = controller.load(false)
    await vi.waitFor(() => expect(decode).toBeTypeOf("function"))
    controller.stop()
    expect(fetch.mock.calls.at(-1)![1]?.signal?.aborted).toBe(true)
    decode({ ...group, name: "Obsolete response" })
    await pending
    expect(controller.state.group).toEqual(group)
  })

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
  it("preserves metadata drafts and their revision while changing membership", async () => {
    const fetch = transport()
    vi.stubGlobal("fetch", fetch)
    const { controller } = fixture()
    await controller.load()
    controller.state.form.name = "Unsaved draft"
    await controller.toggleMember({
      principal: "prn_alice",
      enabled: true,
      principal_enabled: true,
      revision: 3,
    })
    expect(controller.state.form.name).toBe("Unsaved draft")
    expect(controller.state.form.revision).toBe(1)
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/system/workspaces/wsp_team/groups/grp_developers/members/prn_alice",
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
  it("adds and toggles bot memberships independently of human memberships and metadata drafts", async () => {
    const fetch = transport()
    vi.stubGlobal("fetch", fetch)
    const { controller, onAgentsChanged, onMembersChanged } = fixture()
    await controller.load()
    controller.state.form.name = "Unsaved group draft"
    controller.state.agent = "wag_assistant"
    await controller.addAgent()
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/system/workspaces/wsp_team/groups/grp_developers/agents",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ agent: "wag_assistant" }),
      }),
    )
    expect(controller.state.agent).toBe("")
    await controller.toggleAgent({
      agent: "wag_assistant",
      alias: "assistant",
      agent_enabled: true,
      enabled: true,
      revision: 4,
    })
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/system/workspaces/wsp_team/groups/grp_developers/agents/wag_assistant",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({ enabled: false, expected_revision: 4 }),
      }),
    )
    await controller.toggleAgent({
      agent: "wag_assistant",
      alias: "assistant",
      agent_enabled: true,
      enabled: false,
      revision: 5,
    })
    expect(fetch).toHaveBeenLastCalledWith(
      "/api/v1/system/workspaces/wsp_team/groups/grp_developers/agents/wag_assistant",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({ enabled: true, expected_revision: 5 }),
      }),
    )
    expect(onAgentsChanged).toHaveBeenCalledTimes(3)
    expect(onMembersChanged).not.toHaveBeenCalled()
    expect(controller.state.form.name).toBe("Unsaved group draft")
    expect(controller.state.form.revision).toBe(1)
    controller.stop()
  })
  it("retains bot choices on duplicate or failed additions and reloads stale memberships without retrying", async () => {
    const fetch = transport()
    vi.stubGlobal("fetch", fetch)
    const { controller, onAgentsChanged } = fixture()
    controller.state.agent = "wag_assistant"
    fetch.mockResolvedValueOnce(
      new Response("membership already exists", { status: 409 }),
    )
    await controller.addAgent()
    expect(controller.state.agent).toBe("wag_assistant")
    expect(controller.state.error).toContain("already exists")
    expect(onAgentsChanged).not.toHaveBeenCalled()
    fetch.mockResolvedValueOnce(
      new Response("This record changed elsewhere. Reload before retrying.", {
        status: 409,
      }),
    )
    await controller.toggleAgent({
      agent: "wag_assistant",
      alias: "assistant",
      agent_enabled: true,
      enabled: true,
      revision: 1,
    })
    expect(onAgentsChanged).toHaveBeenCalledOnce()
    expect(fetch).toHaveBeenCalledTimes(2)
    controller.stop()
  })
  it("aborts an in-flight bot addition and ignores its response after leaving the page", async () => {
    let resolve!: (value: Response) => void
    const fetch = vi.fn<typeof globalThis.fetch>(
      () =>
        new Promise<Response>((done) => {
          resolve = done
        }),
    )
    vi.stubGlobal("fetch", fetch)
    const { controller, onAgentsChanged } = fixture()
    controller.state.agent = "wag_assistant"
    const pending = controller.addAgent()
    controller.stop()
    expect(fetch.mock.calls[0][1]?.signal?.aborted).toBe(true)
    resolve(new Response(null, { status: 201 }))
    await pending
    expect(onAgentsChanged).not.toHaveBeenCalled()
    expect(controller.state.notice).toBe("")
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
