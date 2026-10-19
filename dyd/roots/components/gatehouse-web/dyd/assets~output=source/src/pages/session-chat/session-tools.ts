import { routePath, type Route } from "../../route"

export type SessionWorkspaceRoute = Extract<Route, { sessionID: string }>
export type ConversationRoute = Extract<Route, { kind: "session-chat" }>
export type SessionTool = "files" | "notes" | "tasks" | "secrets"

export function sessionTool(route: SessionWorkspaceRoute): SessionTool | null {
  if (route.kind === "session-files") return "files"
  if (route.kind.startsWith("session-note")) return "notes"
  if (route.kind.startsWith("session-task")) return "tasks"
  if (route.kind.startsWith("session-secret")) return "secrets"
  return null
}

export function sessionToolBack(route: SessionWorkspaceRoute): {
  path: string
  label: string
} | undefined {
  const base = routePath({
    kind: "session-chat",
    workspaceID: route.workspaceID,
    sessionID: route.sessionID,
  })
  switch (route.kind) {
    case "session-note-edit":
    case "session-note-history":
    case "session-note-revision":
      return { path: `${base}/notes/${encodeURIComponent(route.noteID)}`, label: "Back to note" }
    case "session-task-edit":
      return { path: `${base}/tasks/${encodeURIComponent(route.taskID)}`, label: "Back to task" }
    case "session-secret-edit":
      return { path: `${base}/secrets/${encodeURIComponent(route.secretID)}`, label: "Back to secret" }
    case "session-note":
    case "session-note-new":
      return { path: `${base}/notes`, label: "Back to notes" }
    case "session-task":
    case "session-task-new":
      return { path: `${base}/tasks`, label: "Back to tasks" }
    case "session-secret":
    case "session-secret-new":
      return { path: `${base}/secrets`, label: "Back to secrets" }
    default:
      return undefined
  }
}
