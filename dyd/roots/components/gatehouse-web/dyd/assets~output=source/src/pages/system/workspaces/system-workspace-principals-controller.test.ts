import { afterEach, describe, expect, it, vi } from "vitest"
import { clearAccessToken } from "../../../app/api"
import { createSystemWorkspacePrincipalsController } from "./system-workspace-principals-controller.svelte"
import { createSystemWorkspacePrincipalNewController } from "./system-workspace-principal-new-controller.svelte"

afterEach(() => {
  clearAccessToken()
  vi.unstubAllGlobals()
})

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  })
}

describe("workspace principal controllers", () => {
  it("loads direct principals in pages and removes direct access", async () => {
    const workspaceID = "wsp_01abc"
    const alice = {
      id: "prn_alice",
      alias: "alice",
      name: "Alice",
      enabled: true,
      roles: ["guest"],
    }
    const bob = {
      id: "prn_bob",
      alias: "bob",
      name: "Bob",
      enabled: true,
      roles: ["member"],
    }
    const request = vi.fn(
      async (
        input: RequestInfo | URL,
        init?: RequestInit,
      ): Promise<Response> => {
        const url = String(input)
        if (init?.method === "DELETE")
          return new Response(null, { status: 204 })
        if (url.includes("cursor=prn_cursor"))
          return jsonResponse({ principals: [bob] })
        return jsonResponse({ principals: [alice], next_cursor: "prn_cursor" })
      },
    )
    vi.stubGlobal("fetch", request)
    vi.stubGlobal("window", { confirm: vi.fn(() => true) })

    const controller = createSystemWorkspacePrincipalsController({
      onAuthenticationLost: vi.fn(),
      onSystemAccessChange: vi.fn(),
    })
    const stop = controller.load(workspaceID)
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(controller.state.principals).toEqual([alice])
    expect(controller.state.cursor).toBe("prn_cursor")

    controller.loadMore(workspaceID)
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(
      controller.state.principals.map((principal) => principal.id),
    ).toEqual([alice.id, bob.id])
    const bobRow = controller.state.principals.find(
      (principal) => principal.id === bob.id,
    )
    if (bobRow === undefined)
      throw new Error("expected Bob in workspace roster")
    await controller.remove(workspaceID, bobRow)
    expect(
      controller.state.principals.map((principal) => principal.id),
    ).toEqual([alice.id])
    expect(
      request.mock.calls.some(
        ([url, init]) =>
          String(url).endsWith(`/principals/${bob.id}`) &&
          init?.method === "DELETE",
      ),
    ).toBe(true)
    stop()
  })

  it("searches picker pages and submits the chosen role", async () => {
    const workspaceID = "wsp_01abc"
    const bob = {
      id: "prn_bob",
      alias: "bob",
      name: "Bob",
      enabled: true,
    }
    const carol = {
      id: "prn_carol",
      alias: "carol",
      name: "Carol",
      enabled: true,
    }
    const request = vi.fn(
      async (
        input: RequestInfo | URL,
        init?: RequestInit,
      ): Promise<Response> => {
        const url = String(input)
        if (init?.method === "POST") return jsonResponse({}, 201)
        if (url.includes("cursor=prn_cursor"))
          return jsonResponse({ principals: [carol] })
        return jsonResponse({ principals: [bob], next_cursor: "prn_cursor" })
      },
    )
    vi.stubGlobal("fetch", request)

    const controller = createSystemWorkspacePrincipalNewController({
      onAuthenticationLost: vi.fn(),
      onSystemAccessChange: vi.fn(),
    })
    const stop = controller.load()
    controller.state.search = "b"
    await controller.search(workspaceID)
    expect(controller.state.principals).toEqual([bob])
    expect(controller.state.cursor).toBe("prn_cursor")
    await controller.search(workspaceID, true)
    expect(
      controller.state.principals.map((principal) => principal.id),
    ).toEqual([bob.id, carol.id])

    controller.state.selectedPrincipalID = bob.id
    controller.state.role = "member"
    await expect(controller.add(workspaceID)).resolves.toBe(true)
    expect(
      request.mock.calls.some(
        ([url, init]) =>
          String(url).endsWith(`/workspaces/${workspaceID}/principals`) &&
          init?.method === "POST" &&
          init.body ===
            JSON.stringify({ principal: bob.id, role: "member" }),
      ),
    ).toBe(true)
    stop()
  })
})
