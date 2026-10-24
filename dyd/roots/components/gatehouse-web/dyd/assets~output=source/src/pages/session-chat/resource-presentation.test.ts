import { describe, expect, it } from "vitest"
import { importClientModule } from "../../testing/client-module"
import { parseRoute } from "../../route"
import { sessionToolDismissPath, type SessionWorkspaceRoute } from "./session-tools"

const conversation = { kind: "session-chat", workspaceID: "workspace", sessionID: "chat" } as const
const note = { ...conversation, kind: "session-note", noteID: "note" } as const

async function create() {
  const { createResourcePresentation } = await importClientModule(
    new URL("./resource-presentation.svelte.ts", import.meta.url),
  )
  return createResourcePresentation()
}

describe("resource dialog presence", () => {
  it("dismisses a reader to its list before closing the tool from the list", async () => {
    const presentation = await create()
    presentation.update(note, true, "principal")
    const parent = parseRoute(new URL(
      sessionToolDismissPath(note, conversation),
      "http://localhost",
    )) as SessionWorkspaceRoute
    presentation.update(parent, true, "principal")
    expect(presentation.state.open).toBe(true)
    expect(presentation.state.route).toEqual({ ...conversation, kind: "session-notes" })

    const closed = parseRoute(new URL(
      sessionToolDismissPath(parent, conversation),
      "http://localhost",
    )) as SessionWorkspaceRoute
    presentation.update(closed, true, "principal")
    expect(presentation.state.open).toBe(false)
    expect(presentation.state.route).toEqual(parent)
    presentation.closed()
    expect(presentation.state.route).toBeNull()
  })

  it.each([
    { editor: { ...note, kind: "session-note-edit" }, expected: note },
    { editor: { ...conversation, kind: "session-note-new" }, expected: { ...conversation, kind: "session-notes" } },
  ] as const)("dismisses $editor.kind to its parent without closing the dialog", async ({ editor, expected }) => {
    const presentation = await create()
    presentation.update(editor, true, "principal")
    const parent = parseRoute(new URL(
      sessionToolDismissPath(editor, conversation),
      "http://localhost",
    )) as SessionWorkspaceRoute
    presentation.update(parent, true, "principal")
    expect(presentation.state.open).toBe(true)
    expect(presentation.state.route).toEqual(expected)

    presentation.closed()
    expect(presentation.state.route).toEqual(parent)
  })

  it("retains the displayed note until closing completes, without reverting to a collection", async () => {
    const presentation = await create()
    presentation.update(note, true, "principal")
    presentation.update(conversation, true, "principal")
    expect(presentation.state.open).toBe(false)
    expect(presentation.state.route).toEqual(note)

    presentation.closed()
    expect(presentation.state.route).toBeNull()
  }, 30000)

  it("does not let an obsolete close completion remove a reopened resource", async () => {
    const presentation = await create()
    const files = { ...conversation, kind: "session-files" }
    presentation.update(note, true, "principal")
    presentation.update(conversation, true, "principal")
    presentation.update(files, true, "principal")
    presentation.closed()
    expect(presentation.state.open).toBe(true)
    expect(presentation.state.route).toEqual(files)
  })

  it("releases closing content immediately on session or principal changes", async () => {
    const presentation = await create()
    presentation.update(note, true, "principal")
    presentation.update({ ...conversation, sessionID: "another-chat" }, true, "principal")
    expect(presentation.state.route).toBeNull()
    presentation.update(note, true, "principal")
    presentation.update(conversation, true, "another-principal")
    expect(presentation.state.route).toBeNull()
  })

  it("does not retain resource content after access is lost", async () => {
    const presentation = await create()
    presentation.update(note, true, "principal")
    presentation.update(note, false, "principal")
    expect(presentation.state.open).toBe(false)
    expect(presentation.state.route).toBeNull()
  })
})
