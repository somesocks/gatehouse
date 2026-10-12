import { describe, expect, it, vi } from "vitest"
import { importClientModule } from "../../testing/client-module"

const form = {
  version: 1,
  type: "form",
  title: "Review",
  fields: [{ id: "name", label: "Name", type: "text" }],
}
function transport() {
  return {
    read: vi.fn(async () => Response.json({ form, draft: {} })),
    patch: vi.fn(async () => new Response(null, { status: 204 })),
    terminal: vi.fn(async () => new Response(null, { status: 202 })),
  }
}

describe("shared form reload and disposal", () => {
  it("aborts in-flight uploads after confirmed access loss", async () => {
    const { createInputFormController } = await importClientModule(
      new URL("./input-controller.svelte.ts", import.meta.url),
    )
    const api = Object.assign(transport(), {
      createFile: vi.fn(),
      uploadFile: vi.fn(),
      finishFile: vi.fn(),
    })
    api.read.mockResolvedValueOnce(
      Response.json({
        form: {
          ...form,
          fields: [
            ...form.fields,
            {
              id: "attachments",
              label: "Files",
              type: "files",
              optional: true,
            },
          ],
        },
        draft: {},
      }),
    )
    let finish!: (response: Response) => void
    api.createFile.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const controller = createInputFormController(api)
    await controller.load()
    const uploading = controller.upload(
      ["attachments"],
      [new File(["data"], "report.txt")],
      vi.fn(),
    )
    const signal = api.createFile.mock.calls[0][2] as AbortSignal
    api.read.mockResolvedValueOnce(new Response(null, { status: 403 }))
    await controller.refresh()
    expect(controller.state.status).toBe("denied")
    expect(signal.aborted).toBe(true)
    finish(
      Response.json({
        file: { ref: { id: "obsolete" } },
        upload_url: "/storage",
      }),
    )
    expect(await uploading).toBe(false)
    expect(api.uploadFile).not.toHaveBeenCalled()
    expect(api.terminal).not.toHaveBeenCalled()
  })
  it("keeps a mounted draft after failure and rejects stale text decoding", async () => {
    const { createInputFormController } = await importClientModule(
      new URL("./input-controller.svelte.ts", import.meta.url),
    )
    const api = transport()
    const controller = createInputFormController(api)
    await controller.load()
    await controller.set(["name"], "Local answer")
    const snapshot = controller.state.form
    api.read.mockRejectedValueOnce(new Error("offline"))
    await controller.load()
    expect(controller.state.status).toBe("ready")
    expect(controller.state.form).toBe(snapshot)
    expect(controller.state.draft).toEqual({ name: "Local answer" })
    let decode!: (text: string) => void
    api.read.mockImplementationOnce(async () => ({
      ok: true,
      status: 200,
      text: () =>
        new Promise((resolve) => {
          decode = resolve
        }),
    }))
    const pending = controller.refresh()
    await vi.waitFor(() => expect(decode).toBeTypeOf("function"))
    controller.dispose()
    decode(JSON.stringify({ form, draft: { name: "Obsolete answer" } }))
    await pending
    expect(controller.state.draft).toEqual({ name: "Local answer" })
  })

  it("validates field removal before transport and refuses late terminal validation errors", async () => {
    const { createInputFormController } = await importClientModule(
      new URL("./input-controller.svelte.ts", import.meta.url),
    )
    const api = transport()
    const controller = createInputFormController(api)
    await controller.load()
    expect(await controller.remove(["unknown"])).toBe(false)
    expect(api.patch).not.toHaveBeenCalled()
    await controller.set(["name"], "Local answer")
    let decode!: (text: string) => void
    api.terminal.mockImplementationOnce(async () => ({
      ok: false,
      status: 422,
      text: () =>
        new Promise((resolve) => {
          decode = resolve
        }),
    }))
    const pending = controller.submit()
    await vi.waitFor(() => expect(decode).toBeTypeOf("function"))
    controller.dispose()
    decode("Obsolete error")
    await pending
    expect(controller.state.error).not.toBe("Obsolete error")
    expect(controller.state.draft).toEqual({ name: "Local answer" })
  })
})
