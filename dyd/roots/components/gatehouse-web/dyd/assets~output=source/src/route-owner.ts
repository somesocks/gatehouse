import type { Route } from "./route"

export type RouteOwner = "project-dashboard" | "project-notes" | "legacy"

export function routeOwner(route: Route): RouteOwner {
  switch (route.kind) {
    case "project":
      return "project-dashboard"
    case "project-notes":
    case "project-note-new":
    case "project-note":
      return "project-notes"
    default:
      return "legacy"
  }
}
