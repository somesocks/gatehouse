import type { Route } from "./route"

export type RouteOwner = "project-dashboard" | "project-notes" | "project-secrets" | "session-chat" | "session-notes" | "legacy"

export function routeOwner(route: Route): RouteOwner {
  switch (route.kind) {
    case "project":
      return "project-dashboard"
    case "project-notes":
    case "project-note-new":
    case "project-note":
      return "project-notes"
    case "project-secrets":
    case "project-secret-new":
    case "project-secret":
      return "project-secrets"
    case "session-chat":
      return "session-chat"
    case "session-notes":
    case "session-note-new":
    case "session-note":
    case "session-note-edit":
    case "session-note-revision":
      return "session-notes"
    default:
      return "legacy"
  }
}
