import { afterEach, describe, expect, it, vi } from "vitest"
import { importClientModule } from "../../testing/client-module"

afterEach(() => vi.unstubAllGlobals())

describe("administration edit snapshots and baselines", () => {
  it("preserves a provider draft and its expected revision after a conflicting write", async () => {
    const { createAgentProviderFormController } = await importClientModule(
      new URL(
        "./agent-providers/agent-provider-form-controller.svelte.ts",
        import.meta.url,
      ),
    )
    const first = {
      id: "provider",
      alias: "Original",
      revision: 1,
      protocol: "builtin",
      enabled: true,
    }
    const latest = { ...first, alias: "Changed elsewhere", revision: 2 }
    let snapshot = first
    const request = vi.fn(async (url: string, options?: RequestInit) =>
      options?.method === "PATCH"
        ? new Response(null, { status: 409 })
        : Response.json(url.endsWith("/keychains") ? [] : snapshot),
    )
    vi.stubGlobal("fetch", request)
    const controller = createAgentProviderFormController({
      onAuthenticationLost: vi.fn(),
      onSystemAccessChange: vi.fn(),
    })
    await controller.load("provider")
    await controller.beginEdit()
    const form = controller.state.form
    form.alias = "Unsaved local alias"
    snapshot = latest
    expect(await controller.update()).toBe(false)
    expect(controller.state.loading).toBe(false)
    expect(controller.state.provider).toEqual(latest)
    expect(controller.state.form).toBe(form)
    expect(controller.state.form.alias).toBe("Unsaved local alias")
    await controller.update()
    const patches = request.mock.calls.filter(
      ([, options]) => options?.method === "PATCH",
    )
    expect(
      patches.map(
        ([, options]) => JSON.parse(options!.body as string).expected_revision,
      ),
    ).toEqual([1, 1])
    await controller.replaceDraft()
    expect(controller.state.form.alias).toBe("Changed elsewhere")
    expect(controller.state.conflict).toBe(false)
    controller.stop()
  })

  it("rejects late decoded model data and obsolete authentication errors after an owner change or disposal", async () => {
    const { createAgentModelFormController } = await importClientModule(
      new URL(
        "./agent-models/agent-model-form-controller.svelte.ts",
        import.meta.url,
      ),
    )
    let decoded!: (value: object) => void
    const request = vi.fn(async () => ({
      status: 200,
      ok: true,
      json: () =>
        new Promise((resolve) => {
          decoded = resolve
        }),
    }))
    vi.stubGlobal("fetch", request)
    const authenticationLost = vi.fn()
    const controller = createAgentModelFormController({
      onAuthenticationLost: authenticationLost,
      onSystemAccessChange: vi.fn(),
    })
    const old = controller.load("old")
    await vi.waitFor(() => expect(decoded).toBeTypeOf("function"))
    request.mockImplementationOnce(async () =>
      Response.json({ id: "current", alias: "Current", revision: 1 }),
    )
    await controller.load("current")
    decoded({ id: "old", alias: "Obsolete", revision: 1 })
    await old
    expect(controller.state.model.id).toBe("current")
    let finish!: (response: Response) => void
    request.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const pending = controller.load("current")
    controller.stop()
    finish(new Response(null, { status: 401 }))
    await pending
    expect(authenticationLost).not.toHaveBeenCalled()
    expect(controller.state.error).toBe("")
    expect(controller.state.model.id).toBe("current")
  })
})
