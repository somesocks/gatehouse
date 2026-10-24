import { describe, expect, it } from "vitest"
import { sessionToolDismissPath } from "./session-tools"

const conversation = {
  kind: "session-chat",
  workspaceID: "workspace/one",
  sessionID: "chat/one",
} as const
const base = "/app/wsp/workspace%2Fone/ses/chat%2Fone"

describe("chat-resource dismissal", () => {
  it.each([
    { route: { ...conversation, kind: "session-note-new" }, parent: "notes" },
    { route: { ...conversation, kind: "session-task-new" }, parent: "tasks" },
    { route: { ...conversation, kind: "session-secret-new" }, parent: "secrets" },
    { route: { ...conversation, kind: "session-note", noteID: "note" }, parent: "notes" },
    { route: { ...conversation, kind: "session-task", taskID: "task" }, parent: "tasks" },
    { route: { ...conversation, kind: "session-secret", secretID: "secret" }, parent: "secrets" },
  ] as const)("returns $route.kind to its collection rather than dismissing all chat tools", ({ route, parent }) => {
    expect(sessionToolDismissPath(route, conversation)).toBe(`${base}/${parent}`)
    expect(sessionToolDismissPath(route)).toBe(`${base}/${parent}`)
  })

  it.each([
    { route: { ...conversation, kind: "session-note-edit", noteID: "note/one" }, parent: "notes/note%2Fone" },
    { route: { ...conversation, kind: "session-task-edit", taskID: "task/one" }, parent: "tasks/task%2Fone" },
    { route: { ...conversation, kind: "session-secret-edit", secretID: "secret/one" }, parent: "secrets/secret%2Fone" },
  ] as const)("returns $route.kind to the item being edited", ({ route, parent }) => {
    expect(sessionToolDismissPath(route, conversation)).toBe(`${base}/${parent}`)
    expect(sessionToolDismissPath(route)).toBe(`${base}/${parent}`)
  })

  it.each([
    { ...conversation, kind: "session-files" },
    { ...conversation, kind: "session-notes" },
    { ...conversation, kind: "session-tasks" },
    { ...conversation, kind: "session-secrets" },
  ] as const)("dismisses $kind to the conversation", (route) => {
    expect(sessionToolDismissPath(route, conversation)).toBe(base)
  })

  it.each([
    { ...conversation, kind: "session-note-history", noteID: "note/one" },
    { ...conversation, kind: "session-note-revision", noteID: "note/one", revision: 2 },
  ] as const)("returns $kind to the note", (route) => {
    expect(sessionToolDismissPath(route, conversation)).toBe(`${base}/notes/note%2Fone`)
  })
})
