import type { Route } from "./route"

export type RouteOwner = "project-dashboard" | "project-notes" | "project-secrets" | "session-chat" | "session-notes" | "session-secrets" | "chat-collection" | "project-collection" | "groups" | "system-grants" | "principals" | "agent-providers" | "agent-models" | "storage-providers" | "workspace-agent-bindings" | "workspace-storage-bindings" | "system" | "workspace-dashboard" | "access"

function unhandledRoute(route: never): never {
  throw new Error(`Unhandled route ${JSON.stringify(route)}`)
}

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
    case "session-secrets":
    case "session-secret-new":
    case "session-secret":
      return "session-secrets"
    case "session-collection":
      return "chat-collection"
    case "project-collection":
      return "project-collection"
    case "group-collection":
      return "groups"
    case "system":
      return "system"
    case "system-grants":
      return "system-grants"
    case "system-principals":
      return "principals"
    case "system-agent-providers":
      return "agent-providers"
    case "system-agent-models":
      return "agent-models"
    case "system-storage-providers":
      return "storage-providers"
    case "system-workspace-agent-bindings":
      return "workspace-agent-bindings"
    case "system-workspace-storage-bindings":
      return "workspace-storage-bindings"
    case "app-home":
    case "workspace":
    case "not-found":
      return "workspace-dashboard"
    case "login":
    case "no-access":
      return "access"
  }
  return unhandledRoute(route)
}
