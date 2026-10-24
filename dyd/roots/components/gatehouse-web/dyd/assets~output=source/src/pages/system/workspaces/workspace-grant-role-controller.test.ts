import { afterEach, describe, expect, it, vi } from "vitest"
import { clearAccessToken } from "../../../app/api"
import type { SystemWorkspacePrincipalGrant } from "../../../app/system"
import { createWorkspaceGrantRoleController } from "./workspace-grant-role-controller.svelte"

afterEach(() => {
  clearAccessToken()
  vi.unstubAllGlobals()
})

const grant: SystemWorkspacePrincipalGrant = {
  id: "wgr_one",
  workspace: "wsp_team",
  principal: "prn_alice",
  role: "guest",
  enabled: true,
  revision: 3,
}
function fixture() {
  const onReload = vi.fn(async () => true)
  const onAuthenticationLost = vi.fn(),
    onSystemAccessChange = vi.fn()
  const controller = createWorkspaceGrantRoleController({
    workspaceID: () => "wsp_team",
    onReload,
    onAuthenticationLost,
    onSystemAccessChange,
  })
  controller.begin(grant, "Alice")
  return { controller, onReload, onAuthenticationLost, onSystemAccessChange }
}

describe("workspace grant role editor", () => {
  it("updates only the selected grant's role using its revision and refreshes the roster", async () => {
    const fetch = vi.fn<typeof globalThis.fetch>(async () =>
      Response.json({ ...grant, role: "member", revision: 4 }),
    )
    vi.stubGlobal("fetch", fetch)
    const { controller, onReload } = fixture()
    controller.state.role = "member"
    await controller.save()
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/system/workspaces/wsp_team/grants/wgr_one",
      expect.objectContaining({
        method: "PATCH",
        body: JSON.stringify({ role: "member", expected_revision: 3 }),
      }),
    )
    expect(onReload).toHaveBeenCalledOnce()
    expect(controller.state.grant).toBeNull()
    expect(controller.state.notice).toMatch(/updated/)
    expect(grant.role).toBe("guest")
    controller.stop()
  })

  it("preserves the draft on role collisions and allows choosing another role", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            "Another grant for this recipient already has that role, including disabled grants.",
            { status: 409 },
          ),
      ),
    )
    const { controller, onReload } = fixture()
    controller.state.role = "admin"
    await controller.save()
    expect(controller.state.error).toMatch(/Another grant/)
    expect(controller.state.conflict).toBe(false)
    expect(controller.state.role).toBe("admin")
    expect(controller.state.grant?.id).toBe(grant.id)
    expect(onReload).toHaveBeenCalledOnce()
    controller.cancel()
    expect(controller.state.grant).toBeNull()
    controller.stop()
  })

  it("requires reopening a stale grant while preserving the role draft", async () => {
    const fetch = vi.fn(
      async () =>
        new Response(
          "This grant changed elsewhere. Reload it before retrying.",
          { status: 409 },
        ),
    )
    vi.stubGlobal("fetch", fetch)
    const { controller, onReload } = fixture()
    controller.state.role = "member"
    await controller.save()
    expect(controller.state.conflict).toBe(true)
    expect(controller.state.role).toBe("member")
    expect(controller.state.error).toMatch(/Cancel and reopen/)
    await controller.save()
    expect(fetch).toHaveBeenCalledOnce()
    expect(onReload).toHaveBeenCalledOnce()
    controller.cancel()
    controller.begin({ ...grant, revision: 4 }, "Alice")
    expect(controller.state.conflict).toBe(false)
    expect(controller.state.grant?.revision).toBe(4)
    controller.stop()
  })

  it.each([401, 403])(
    "handles lost administrative access (%s)",
    async (status) => {
      vi.stubGlobal(
        "fetch",
        vi.fn(async () => new Response(null, { status })),
      )
      const {
        controller,
        onAuthenticationLost,
        onSystemAccessChange,
        onReload,
      } = fixture()
      await controller.save()
      if (status === 401) expect(onAuthenticationLost).toHaveBeenCalledOnce()
      else expect(onSystemAccessChange).toHaveBeenCalledWith("denied")
      expect(onReload).not.toHaveBeenCalled()
      controller.stop()
    },
  )

  it("aborts a pending save and ignores its late response after leaving the page", async () => {
    let resolve!: (value: Response) => void
    const fetch = vi.fn<typeof globalThis.fetch>(
      () =>
        new Promise<Response>((done) => {
          resolve = done
        }),
    )
    vi.stubGlobal("fetch", fetch)
    const { controller, onReload } = fixture()
    const pending = controller.save()
    controller.stop()
    expect(fetch.mock.calls[0][1]?.signal?.aborted).toBe(true)
    resolve(Response.json({ ...grant, revision: 4 }))
    await pending
    expect(onReload).not.toHaveBeenCalled()
    expect(controller.state.grant).toBeNull()
    expect(controller.state.notice).toBe("")
  })
})
