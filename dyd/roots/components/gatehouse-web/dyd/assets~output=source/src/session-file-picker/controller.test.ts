import { describe, expect, it } from "vitest"
import { createSessionFilePicker, type SessionFile } from "./controller.svelte"
import type { FieldTransport } from "./api"
import { fileBrowseFromSearch, type FileBrowse } from "./browse"

const first: SessionFile = { id: "sfi_one", name: "report.txt", size: 4, media_type: "text/plain" }
const second: SessionFile = { id: "sfi_two", name: "older.txt", size: 8, media_type: "text/plain" }

function fixture(capabilities = ["session.file.list", "session.file.read", "session.file.upload"]) {
  const saved: string[][] = []
  const queries: { browse: FileBrowse; accept: string[] }[] = []
  const transport: FieldTransport = {
    read: async () => Response.json({ label: "Attachments", inputs: { media_types: ["text/plain"] }, capabilities }),
    files: async (browse, accept) => {
      queries.push({ browse: { ...browse }, accept })
      return Response.json(browse.cursor === first.id && browse.direction === "next"
        ? { files: [second], previous_cursor: second.id }
        : { files: [first], next_cursor: first.id })
    },
    references: async (ids) => Response.json({ files: [first, second].filter((file) => ids.includes(file.id)) }),
    save: async (ids) => { saved.push(ids); return new Response(null, { status: 204 }) },
    createFile: async () => Response.json({ file: { ref: { id: "sfi_uploaded" } }, upload_url: "/storage" }),
    upload: async () => new Response(null, { status: 204 }),
    finishFile: async () => Response.json({ id: "sfi_uploaded", name: "new.txt", size: 4, media_type: "text/plain" }),
    download: async () => Response.json({ url: "/storage/download" }),
  }
  return { transport, saved, queries }
}

describe("session file picker", () => {
  it("pluralizes the stock singular picker label", async () => {
    const server = fixture()
    server.transport.read = async () => Response.json({ label: "Select a session file", inputs: {}, capabilities: [] })
    const picker = createSessionFilePicker(server.transport)
    await picker.load()
    expect(picker.state.label).toBe("Select session files")
  })

  it("requests the URL's filters and a bounded page, retaining selections across pages", async () => {
    const server = fixture()
    const initial = fileBrowseFromSearch("?name=report&media_type=text%2Fplain&limit=10")
    const picker = createSessionFilePicker(server.transport, initial)
    await picker.load()
    expect(server.queries).toEqual([{ browse: initial, accept: ["text/plain"] }])
    picker.select(first)
    await picker.browse({ ...initial, cursor: first.id, direction: "next" })
    expect(picker.state.files).toEqual([second])
    picker.select(second)
    expect(picker.state.selected.map((file) => file.id)).toEqual([first.id, second.id])
    expect(await picker.save()).toBe(true)
    expect(picker.state.saved).toBe(true)
    expect(server.saved).toEqual([[first.id, second.id]])
    picker.remove(first.id)
    await picker.save()
    expect(server.saved[1]).toEqual([second.id])
    picker.remove(second.id)
    await picker.save()
    expect(server.saved[2]).toEqual([])
    server.transport.save = async () => new Response(null, { status: 409 })
    expect(await picker.save()).toBe(false)
    expect(picker.state.status).toBe("resolved")
  })

  it("does not let a failed save reuse an earlier success", async () => {
    const server = fixture()
    const picker = createSessionFilePicker(server.transport)
    await picker.load()
    picker.select(first)
    expect(await picker.save()).toBe(true)
    expect(picker.state.saved).toBe(true)

    server.transport.save = async () => new Response(null, { status: 500 })
    expect(await picker.save()).toBe(false)
    expect(picker.state.saved).toBe(false)
    expect(picker.state.error).toContain("500")
  })

  it("hydrates saved selections without fetching other pages", async () => {
    const server = fixture()
    server.transport.read = async () => Response.json({ label: "Files", inputs: {}, capabilities: ["session.file.list"], value: { file_ids: [second.id, first.id] } })
    const picker = createSessionFilePicker(server.transport)
    await picker.load()
    expect(picker.state.selected).toEqual([second, first])
    expect(server.queries).toHaveLength(1)
    picker.select(first)
    expect(picker.state.selected).toEqual([second])
  })

  it("handles unavailable saved files without dropping their selection silently", async () => {
    const server = fixture()
    server.transport.read = async () => Response.json({ label: "Files", inputs: {}, capabilities: ["session.file.list"], value: { file_ids: ["sfi_removed"] } })
    const picker = createSessionFilePicker(server.transport)
    await picker.load()
    expect(picker.state.selected[0].unavailable).toBe(true)
    await picker.save()
    expect(server.saved).toEqual([])
    picker.remove("sfi_removed")
    await picker.save()
    expect(server.saved).toEqual([[]])
  })

  it("finishes uploads and selects them without overflowing the current page", async () => {
    const server = fixture()
    const picker = createSessionFilePicker(server.transport)
    await picker.load()
    await picker.upload([new File(["data"], "new.txt", { type: "text/plain" })])
    expect(picker.state.selected.map((file) => file.id)).toEqual(["sfi_uploaded"])
    expect(picker.state.files).toEqual([first])
    await picker.save()
    expect(server.saved).toEqual([["sfi_uploaded"]])
  })

  it("supports upload-only capabilities without requesting the file listing", async () => {
    const server = fixture(["session.file.upload"])
    const picker = createSessionFilePicker(server.transport)
    await picker.load()
    expect(server.queries).toHaveLength(0)

    await picker.upload([new File(["data"], "new.txt", { type: "text/plain" })])
    expect(picker.state.selected.map((file) => file.id)).toEqual(["sfi_uploaded"])
    expect(await picker.save()).toBe(true)
    expect(server.saved).toEqual([["sfi_uploaded"]])
  })

  it("does not request download credentials until a file is opened", async () => {
    const server = fixture()
    let downloads = 0
    server.transport.download = async () => { downloads += 1; return Response.json({ url: "/storage/download" }) }
    const picker = createSessionFilePicker(server.transport)
    await picker.load()
    picker.select(first)
    expect(downloads).toBe(0)
    expect(await picker.download(first.id)).toBe("/storage/download")
    expect(downloads).toBe(1)
  })

  it("does not invoke operations absent from its delegated capabilities", async () => {
    const server = fixture(["session.file.list"])
    server.transport.download = async () => { throw new Error("must not download") }
    server.transport.createFile = async () => { throw new Error("must not upload") }
    const picker = createSessionFilePicker(server.transport)
    await picker.load()
    picker.select(first)
    expect(await picker.download(first.id)).toBeNull()
    await picker.upload([new File(["data"], "new.txt", { type: "text/plain" })])
    expect(picker.state.error).toBe("")
    expect(picker.state.selected).toEqual([first])
  })

  it("ignores stale page responses when URL navigation changes the search", async () => {
    const server = fixture()
    const picker = createSessionFilePicker(server.transport)
    await picker.load()
    let finish!: (response: Response) => void
    let aborted: AbortSignal | undefined
    server.transport.files = async (_browse, _accept, signal) => {
      aborted = signal
      return new Promise<Response>((resolve) => { finish = resolve })
    }
    const oldPage = picker.browse({ ...picker.state.browse, name: "old" })
    server.transport.files = async () => Response.json({ files: [second] })
    await picker.browse({ ...picker.state.browse, name: "new" })
    expect(aborted?.aborted).toBe(true)
    finish(Response.json({ files: [first] }))
    await oldPage
    expect(picker.state.files).toEqual([second])
    expect(picker.state.browse.name).toBe("new")
  })
})
