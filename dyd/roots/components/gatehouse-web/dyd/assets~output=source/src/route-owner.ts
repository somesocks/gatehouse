import type { Route } from "./route"

export type RouteOwner = "project-dashboard" | "project-notes" | "project-secrets" | "session-chat" | "session-notes" | "session-secrets" | "chat-collection" | "project-collection" | "groups" | "system-overview" | "system-grants" | "principals" | "agent-provider-list" | "agent-provider-new" | "agent-provider-detail" | "agent-model-list" | "agent-model-new" | "agent-model-detail" | "storage-providers" | "workspace-agent-bindings" | "workspace-storage-bindings" | "workspace-dashboard" | "access"

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
      return "system-overview"
    case "system-grants":
      return "system-grants"
    case "system-principals":
      return "principals"
    case "system-agent-providers":
      return "agent-provider-list"
    case "system-agent-provider-new":
      return "agent-provider-new"
    case "system-agent-provider":
      return "agent-provider-detail"
    case "system-agent-models":
      return "agent-model-list"
    case "system-agent-model-new":
      return "agent-model-new"
    case "system-agent-model":
      return "agent-model-detail"
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
