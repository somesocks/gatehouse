import type { Route } from "./route"

export type RouteOwner = "project-notes" | "legacy"

export function routeOwner(route: Route): RouteOwner {
  switch (route.kind) {
    case "project-notes":
    case "project-note-new":
    case "project-note":
      return "project-notes"
    default:
      return "legacy"
  }
}
