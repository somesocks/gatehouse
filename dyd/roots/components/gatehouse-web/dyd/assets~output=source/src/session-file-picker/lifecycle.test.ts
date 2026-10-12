import { describe, expect, it, vi } from "vitest"
import { importClientModule } from "../testing/client-module"

describe("capability file selection lifecycle", () => {
  it("preserves selection and applied browsing on failed reload, and discards late downloads after disposal", async () => {
    const { createSessionFilePicker } = await importClientModule(
      new URL("./controller.svelte.ts", import.meta.url),
    )
    const file = { id: "file", name: "report.txt", media_type: "text/plain" }
    const api = {
      read: vi.fn(async () =>
        Response.json({
          label: "Files",
          inputs: {},
          value: { file_ids: [] },
          capabilities: ["session.file.list", "session.file.read"],
        }),
      ),
      files: vi.fn(async () =>
        Response.json({ files: [file], next_cursor: "second" }),
      ),
      references: vi.fn(async () => Response.json({ files: [file] })),
      download: vi.fn(async () => Response.json({ url: "/download" })),
    }
    const controller = createSessionFilePicker(api)
    await controller.load()
    controller.select(file)
    let finish!: (response: Response) => void
    api.read.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const pending = controller.load()
    expect(controller.state.status).toBe("ready")
    expect(controller.state.reloading).toBe(true)
    finish(new Response(null, { status: 503 }))
    await pending
    expect(controller.state.selected).toEqual([file])
    const browse = controller.state.browse
    api.files.mockResolvedValueOnce(new Response(null, { status: 503 }))
    await controller.browse({ ...browse, name: "new filter", cursor: "second" })
    expect(controller.state.browse).toEqual(browse)
    expect(controller.state.nextCursor).toBe("second")
    let decode!: (value: object) => void
    api.download.mockImplementationOnce(async () => ({
      ok: true,
      status: 200,
      json: () =>
        new Promise((resolve) => {
          decode = resolve
        }),
    }))
    const opening = controller.download(file.id)
    await vi.waitFor(() => expect(decode).toBeTypeOf("function"))
    controller.dispose()
    decode({ url: "/obsolete-download" })
    expect(await opening).toBeNull()
    expect(controller.state.selected).toEqual([file])
  })
})
