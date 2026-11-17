import { afterEach, expect, it, vi } from "vitest"
import { importClientModule } from "../../testing/client-module"

const ref = (id: string) => ({
  id,
  session: { id: "session", workspace: { id: "workspace" } },
})
const message = (id: string) => ({
  ref: ref(id),
  kind: "message.text",
  created_at: "2026-01-01T00:00:00Z",
  payload: { text: id },
})
const page = (ids: string[], extra: object = {}) => ({
  entries: ids.map((id) => ({
    event: message(id),
    previews: [],
    has_more_replies: false,
  })),
  agent_requests: [],
  interactions: [],
  ...extra,
})
async function create(parentID = "") {
  const { createChatController } = await importClientModule(
    new URL("./chat-controller.svelte.ts", import.meta.url),
  )
  const controller = createChatController({
    isNearBottom: () => false,
    followLatest: vi.fn(async () => {}),
    focusComposer: vi.fn(),
  })
  controller.start(
    "workspace",
    "session",
    new AbortController().signal,
    parentID,
  )
  await vi.waitFor(() => expect(controller.state.status).toBe("ready"))
  return controller
}
afterEach(() => vi.unstubAllGlobals())

it("follows opaque cursors on demand and refreshes all opened pages without dropping later entries", async () => {
  const request = vi.fn(async (path: string) => {
    const query = new URL(path, "http://localhost").searchParams
    return Response.json(
      query.has("cursor")
        ? page(["second"])
        : page(["first"], { next_cursor: "opaque +&" }),
    )
  })
  vi.stubGlobal("fetch", request)
  const controller = await create()
  expect(request).toHaveBeenCalledTimes(1)
  controller.state.messageText = "Draft"
  controller.state.sentMessageID = "second"
  expect(await controller.loadMore()).toBe(true)
  expect(controller.state.sentMessageID).toBe("")
  expect(
    controller.state.branch.entries.map((entry) => entry.event.ref.id),
  ).toEqual(["first", "second"])
  expect(
    new URL(request.mock.calls.at(-1)![0], "http://localhost").searchParams.get(
      "cursor",
    ),
  ).toBe("opaque +&")
  expect(await controller.refreshEvents()).toBe(true)
  expect(
    controller.state.branch.entries.map((entry) => entry.event.ref.id),
  ).toEqual(["first", "second"])
  expect(controller.state.messageText).toBe("Draft")
  expect(
    request.mock.calls.every(([path]) =>
      path.includes("/events/branch?limit=20"),
    ),
  ).toBe(true)
  controller.stop()
})

it("invalidates an old branch decode, preserves the root draft/files and treats empty replies as loaded", async () => {
  const request = vi.fn(async () => Response.json(page(["root"])))
  vi.stubGlobal("fetch", request)
  const controller = await create()
  controller.state.messageText = "Unsaved"
  const file = { file: { name: "notes.txt" }, status: "pending" }
  controller.state.composerFiles = [file]
  let decode!: (value: object) => void
  request.mockImplementationOnce(
    async () =>
      ({
        status: 200,
        ok: true,
        json: () =>
          new Promise((resolve) => {
            decode = resolve
          }),
      }) as Response,
  )
  const old = controller.refreshEvents()
  await vi.waitFor(() => expect(decode).toBeTypeOf("function"))
  request.mockResolvedValueOnce(
    Response.json(page([], { parent: message("answer") })),
  )
  controller.selectBranch("answer")
  await vi.waitFor(() => expect(controller.state.status).toBe("ready"))
  decode(page(["obsolete"]))
  expect(await old).toBe(false)
  expect(controller.state.parentID).toBe("answer")
  expect(controller.state.branch.entries).toEqual([])
  expect(controller.state.branch.parent?.ref.id).toBe("answer")
  expect(controller.state.branch.parent?.payload).toEqual({ text: "answer" })
  expect(controller.state.messageText).toBe("Unsaved")
  expect(controller.state.composerFiles[0].file.name).toBe("notes.txt")
  controller.selectBranch()
  await vi.waitFor(() => expect(controller.state.status).toBe("ready"))
  expect(controller.state.branch.entries[0].event.ref.id).toBe("root")
  expect(controller.state.messageText).toBe("Unsaved")
  controller.stop()
})

it("does not insert a completed root send into a branch opened during the send", async () => {
  let finish!: (response: Response) => void
  const sent: object[] = []
  const request = vi.fn(async (path: string, init?: RequestInit) => {
    if (path.endsWith("/messages")) {
      sent.push(JSON.parse(String(init?.body)))
      return new Promise<Response>((resolve) => {
        finish = resolve
      })
    }
    const parent = new URL(path, "http://localhost").searchParams.get("parent")
    return Response.json(page([], parent ? { parent: message(parent) } : {}))
  })
  vi.stubGlobal("fetch", request)
  const controller = await create()
  controller.state.messageText = "Root message"
  const sending = controller.sendMessage()
  await vi.waitFor(() => expect(finish).toBeTypeOf("function"))
  controller.selectBranch("answer")
  await vi.waitFor(() => expect(controller.state.status).toBe("ready"))
  controller.state.messageText = "Next draft"
  finish(Response.json(message("new-root")))
  await sending
  expect(sent).toEqual([{ text: "Root message", targets: [] }])
  expect(controller.state.branch.entries).toEqual([])
  expect(controller.state.messageText).toBe("Next draft")
  expect(controller.state.sentMessageID).toBe("new-root")
  controller.selectBranch("new-root")
  await vi.waitFor(() => expect(controller.state.status).toBe("ready"))
  expect(controller.state.sentMessageID).toBe("")
  controller.stop()
})

it("captures a reply's parent, recipients and files before upload even when navigating back to root", async () => {
  let finishUpload!: (response: Response) => void
  const sent: object[] = []
  const request = vi.fn(
    async (path: string, init?: RequestInit): Promise<Response> => {
      if (path.endsWith("/files"))
        return new Promise<Response>((resolve) => {
          finishUpload = resolve
        })
      if (path === "/upload" || path.endsWith("/finish"))
        return new Response(null, { status: 204 })
      if (path.endsWith("/messages")) {
        const body = JSON.parse(String(init?.body))
        sent.push(body)
        return Response.json({
          ...message("sent-reply"),
          parent: ref(body.parent),
        })
      }
      const parent = new URL(path, "http://localhost").searchParams.get(
        "parent",
      )
      return Response.json(page([], parent ? { parent: message(parent) } : {}))
    },
  )
  vi.stubGlobal("fetch", request)
  const controller = await create("answer")
  controller.state.messageText = "Reply with a file"
  const file = new File(["notes"], "notes.txt", { type: "text/plain" })
  controller.state.composerFiles = [{ file, status: "pending" }]
  const targets = [{ kind: "principal", id: "alex" }]
  const sending = controller.sendMessage(targets)
  await vi.waitFor(() => expect(finishUpload).toBeTypeOf("function"))
  await controller.sendMessage(targets)
  controller.selectBranch()
  await vi.waitFor(() => expect(controller.state.status).toBe("ready"))
  controller.state.messageText = "Next draft"
  const nextFile = new File(["later"], "later.txt")
  controller.state.composerFiles = [
    ...controller.state.composerFiles,
    { file: nextFile, status: "pending" },
  ]
  targets.push({ kind: "principal", id: "other" })
  finishUpload(
    Response.json({ file: { ref: { id: "own-file" } }, upload_url: "/upload" }),
  )
  await sending
  expect(sent).toEqual([
    {
      parent: "answer",
      text: "Reply with a file",
      targets: [{ kind: "principal", id: "alex" }],
      attachments: ["own-file"],
    },
  ])
  expect(controller.state.parentID).toBe("")
  expect(controller.state.branch.entries).toEqual([])
  expect(controller.state.messageText).toBe("Next draft")
  expect(controller.state.composerFiles.map((item) => item.file.name)).toEqual([
    "later.txt",
  ])
  expect(controller.state.sentMessageID).toBe("sent-reply")
  expect(
    request.mock.calls.find(([path]) => path.endsWith("/files"))![1]?.signal
      ?.aborted,
  ).toBe(false)
  controller.stop()
})

it("does not send an unavailable thread as a root message", async () => {
  const request = vi.fn(async () => Response.json(page([])))
  vi.stubGlobal("fetch", request)
  const controller = await create()
  request.mockResolvedValueOnce(new Response(null, { status: 404 }))
  controller.selectBranch("missing")
  controller.state.messageText = "Keep this draft"
  await controller.sendMessage()
  await vi.waitFor(() =>
    expect(controller.state.reloadError).toContain("unavailable"),
  )
  await controller.sendMessage()
  expect(controller.state.canCompose).toBe(false)
  expect(controller.state.messageText).toBe("Keep this draft")
  expect(
    request.mock.calls.some(([path]) => String(path).endsWith("/messages")),
  ).toBe(false)
  controller.stop()
})

it("loads and refreshes a full parent on a fresh thread while rejecting a foreign parent", async () => {
  const request = vi.fn(async () =>
    Response.json(page([], { parent: message("answer") })),
  )
  vi.stubGlobal("fetch", request)
  const controller = await create("answer")
  expect(controller.state.branch.parent?.ref.id).toBe("answer")
  expect(controller.state.branch.parent?.payload).toEqual({ text: "answer" })
  request.mockResolvedValueOnce(
    Response.json(
      page([], {
        parent: {
          ...message("answer"),
          payload: { text: "Refreshed context" },
        },
      }),
    ),
  )
  expect(await controller.refreshEvents()).toBe(true)
  expect(controller.state.branch.parent?.payload).toEqual({
    text: "Refreshed context",
  })
  request.mockResolvedValueOnce(
    Response.json(
      page([], {
        parent: {
          ...message("answer"),
          ref: {
            ...ref("answer"),
            session: { id: "foreign", workspace: { id: "workspace" } },
          },
        },
      }),
    ),
  )
  expect(await controller.refreshEvents()).toBe(false)
  expect(controller.state.reloadError).toBe("Invalid conversation branch.")
  expect(controller.state.branch.parent?.payload).toEqual({
    text: "Refreshed context",
  })
  controller.stop()
})

it("retains the last complete read after a failed later page or invalid contract, with no transcript fallback", async () => {
  const request = vi.fn(async () =>
    Response.json(page(["root"], { next_cursor: "next" })),
  )
  vi.stubGlobal("fetch", request)
  const controller = await create()
  request.mockResolvedValueOnce(
    Response.json(page(["newer"], { next_cursor: "next" })),
  )
  request.mockResolvedValueOnce(new Response(null, { status: 503 }))
  expect(await controller.loadMore()).toBe(false)
  expect(controller.state.branch.entries[0].event.ref.id).toBe("root")
  request.mockResolvedValueOnce(Response.json([]))
  expect(await controller.refreshEvents()).toBe(false)
  expect(controller.state.branch.entries[0].event.ref.id).toBe("root")
  expect(controller.state.reloadError).not.toBe("")
  expect(
    request.mock.calls.every(([path]) =>
      String(path).includes("/events/branch?"),
    ),
  ).toBe(true)
  controller.stop()
})
