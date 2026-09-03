type WorkspaceRoute = { workspaceID: string }
type SessionRoute = WorkspaceRoute & { sessionID: string }
type ProjectRoute = WorkspaceRoute & { projectID: string }

export type Route =
  | { kind: "app-home" }
  | { kind: "login"; next: string | null }
  | { kind: "no-access" }
  | ({ kind: "workspace" } & WorkspaceRoute)
  | ({ kind: "session-collection"; search: string } & WorkspaceRoute)
  | ({ kind: "project-collection"; search: string } & WorkspaceRoute)
  | ({ kind: "group-collection" } & WorkspaceRoute)
  | ({ kind: "session-chat" } & SessionRoute)
  | ({ kind: "session-notes" } & SessionRoute)
  | ({ kind: "session-note-new" } & SessionRoute)
  | ({ kind: "session-note"; noteID: string } & SessionRoute)
  | ({ kind: "session-note-edit"; noteID: string } & SessionRoute)
  | ({ kind: "session-note-revision"; noteID: string; revision: number } & SessionRoute)
  | ({ kind: "session-secrets" } & SessionRoute)
  | ({ kind: "session-secret-new" } & SessionRoute)
  | ({ kind: "session-secret"; secretID: string } & SessionRoute)
  | ({ kind: "project" } & ProjectRoute)
  | ({ kind: "project-notes" } & ProjectRoute)
  | ({ kind: "project-note-new" } & ProjectRoute)
  | ({ kind: "project-note"; noteID: string } & ProjectRoute)
  | ({ kind: "project-secrets" } & ProjectRoute)
  | ({ kind: "project-secret-new" } & ProjectRoute)
  | ({ kind: "project-secret"; secretID: string } & ProjectRoute)
  | { kind: "not-found" }

export type NavigableRoute = Exclude<Route, { kind: "not-found" }>

function notFound(): Route {
  return { kind: "not-found" }
}

function pathSegments(pathname: string): string[] | null {
  const trimmed = pathname.length > 1 && pathname.endsWith("/") ? pathname.slice(0, -1) : pathname
  const encoded = trimmed.slice(1).split("/")
  if (encoded.some((segment) => segment === "")) {
    return null
  }
  try {
    return encoded.map(decodeURIComponent)
  } catch {
    return null
  }
}

function collectionSearch(url: URL): string {
  return url.searchParams.get("name") ?? ""
}

export function parseRoute(url: URL): Route {
  const segments = pathSegments(url.pathname)
  if (segments === null || segments[0] !== "app") {
    return notFound()
  }

  if (segments.length === 1) {
    return { kind: "app-home" }
  }
  if (segments.length === 2 && segments[1] === "login") {
    return { kind: "login", next: url.searchParams.get("next") }
  }
  if (segments.length === 2 && segments[1] === "no-access") {
    return { kind: "no-access" }
  }
  if (segments.length < 3 || segments[1] !== "wsp") {
    return notFound()
  }

  const workspaceID = segments[2]
  if (workspaceID === "") {
    return notFound()
  }
  if (segments.length === 3) {
    return { kind: "workspace", workspaceID }
  }

  const collection = segments[3]
  if (segments.length === 4) {
    if (collection === "ses") {
      return { kind: "session-collection", workspaceID, search: collectionSearch(url) }
    }
    if (collection === "prj") {
      return { kind: "project-collection", workspaceID, search: collectionSearch(url) }
    }
    if (collection === "grp") {
      return { kind: "group-collection", workspaceID }
    }
    return notFound()
  }

  const resourceID = segments[4]
  if (resourceID === "") {
    return notFound()
  }
  if (collection === "ses") {
    if (segments.length === 5) {
      return { kind: "session-chat", workspaceID, sessionID: resourceID }
    }
    const section = segments[5]
    if (segments.length === 6 && section === "notes") {
      return { kind: "session-notes", workspaceID, sessionID: resourceID }
    }
    if (segments.length === 7 && section === "notes") {
      return segments[6] === "new"
        ? { kind: "session-note-new", workspaceID, sessionID: resourceID }
        : { kind: "session-note", workspaceID, sessionID: resourceID, noteID: segments[6] }
    }
    if (segments.length === 8 && section === "notes" && segments[7] === "edit" && segments[6] !== "new") {
      return { kind: "session-note-edit", workspaceID, sessionID: resourceID, noteID: segments[6] }
    }
    if (segments.length === 9 && section === "notes" && segments[7] === "revisions" && segments[6] !== "new") {
      const revision = Number(segments[8])
      if (Number.isSafeInteger(revision) && revision > 0 && String(revision) === segments[8]) {
        return { kind: "session-note-revision", workspaceID, sessionID: resourceID, noteID: segments[6], revision }
      }
    }
    if (segments.length === 6 && section === "secrets") {
      return { kind: "session-secrets", workspaceID, sessionID: resourceID }
    }
    if (segments.length === 7 && section === "secrets") {
      return segments[6] === "new"
        ? { kind: "session-secret-new", workspaceID, sessionID: resourceID }
        : { kind: "session-secret", workspaceID, sessionID: resourceID, secretID: segments[6] }
    }
    return notFound()
  }

  if (collection === "prj") {
    if (segments.length === 5) {
      return { kind: "project", workspaceID, projectID: resourceID }
    }
    const section = segments[5]
    if (segments.length === 6 && section === "pnt") {
      return { kind: "project-notes", workspaceID, projectID: resourceID }
    }
    if (segments.length === 7 && section === "pnt") {
      return segments[6] === "new"
        ? { kind: "project-note-new", workspaceID, projectID: resourceID }
        : { kind: "project-note", workspaceID, projectID: resourceID, noteID: segments[6] }
    }
    if (segments.length === 6 && section === "secrets") {
      return { kind: "project-secrets", workspaceID, projectID: resourceID }
    }
    if (segments.length === 7 && section === "secrets") {
      return segments[6] === "new"
        ? { kind: "project-secret-new", workspaceID, projectID: resourceID }
        : { kind: "project-secret", workspaceID, projectID: resourceID, secretID: segments[6] }
    }
  }

  return notFound()
}

function segment(value: string): string {
  return encodeURIComponent(value)
}

function withSearch(pathname: string, search: string): string {
  return search === "" ? pathname : `${pathname}?${new URLSearchParams({ name: search })}`
}

export function routePath(route: NavigableRoute): string {
  switch (route.kind) {
    case "app-home":
      return "/app/"
    case "login":
      return route.next === null ? "/app/login" : `/app/login?${new URLSearchParams({ next: route.next })}`
    case "no-access":
      return "/app/no-access"
    case "workspace":
      return `/app/wsp/${segment(route.workspaceID)}`
    case "session-collection":
      return withSearch(`/app/wsp/${segment(route.workspaceID)}/ses`, route.search)
    case "project-collection":
      return withSearch(`/app/wsp/${segment(route.workspaceID)}/prj`, route.search)
    case "group-collection":
      return `/app/wsp/${segment(route.workspaceID)}/grp`
    case "session-chat":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}`
    case "session-notes":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/notes`
    case "session-note-new":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/notes/new`
    case "session-note":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/notes/${segment(route.noteID)}`
    case "session-note-edit":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/notes/${segment(route.noteID)}/edit`
    case "session-note-revision":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/notes/${segment(route.noteID)}/revisions/${segment(String(route.revision))}`
    case "session-secrets":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/secrets`
    case "session-secret-new":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/secrets/new`
    case "session-secret":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/secrets/${segment(route.secretID)}`
    case "project":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}`
    case "project-notes":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/pnt`
    case "project-note-new":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/pnt/new`
    case "project-note":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/pnt/${segment(route.noteID)}`
    case "project-secrets":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/secrets`
    case "project-secret-new":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/secrets/new`
    case "project-secret":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/secrets/${segment(route.secretID)}`
  }
}

export function routeWorkspaceID(route: Route): string | null {
  return "workspaceID" in route ? route.workspaceID : null
}

export function routeSessionID(route: Route): string | null {
  return "sessionID" in route ? route.sessionID : null
}

export function routeProjectID(route: Route): string | null {
  return "projectID" in route ? route.projectID : null
}

export function routeSessionNoteID(route: Route): string | null {
  if (route.kind === "session-note-new") {
    return "new"
  }
  return route.kind === "session-note" || route.kind === "session-note-edit" || route.kind === "session-note-revision" ? route.noteID : null
}

export function routeSessionNoteRevision(route: Route): number | null {
  return route.kind === "session-note-revision" ? route.revision : null
}

export function routeSessionSecretID(route: Route): string | null {
  if (route.kind === "session-secret-new") {
    return "new"
  }
  return route.kind === "session-secret" ? route.secretID : null
}

export function routeProjectNoteID(route: Route): string | null {
  if (route.kind === "project-note-new") {
    return "new"
  }
  return route.kind === "project-note" ? route.noteID : null
}

export function routeProjectSecretID(route: Route): string | null {
  if (route.kind === "project-secret-new") {
    return "new"
  }
  return route.kind === "project-secret" ? route.secretID : null
}
