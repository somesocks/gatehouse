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

export function sessionToolDismissPath(
  route: SessionWorkspaceRoute,
  conversation: ConversationRoute = {
    kind: "session-chat",
    workspaceID: route.workspaceID,
    sessionID: route.sessionID,
  },
): string {
  const base = routePath({
    kind: "session-chat",
    workspaceID: route.workspaceID,
    sessionID: route.sessionID,
  })
  switch (route.kind) {
    case "session-note-history":
    case "session-note-revision":
    case "session-note-edit":
      return `${base}/notes/${encodeURIComponent(route.noteID)}`
    case "session-task-edit":
      return `${base}/tasks/${encodeURIComponent(route.taskID)}`
    case "session-secret-edit":
      return `${base}/secrets/${encodeURIComponent(route.secretID)}`
    case "session-note":
    case "session-note-new":
      return `${base}/notes`
    case "session-task":
    case "session-task-new":
      return `${base}/tasks`
    case "session-secret":
    case "session-secret-new":
      return `${base}/secrets`
    default:
      return routePath(conversation)
  }
}
