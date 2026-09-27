type WorkspaceRoute = { workspaceID: string }
type SessionRoute = WorkspaceRoute & { sessionID: string }
type ProjectRoute = WorkspaceRoute & { projectID: string }
type ChatMode = "direct" | "group"

export type Route =
  | { kind: "app-home" }
  | { kind: "login"; next: string | null }
  | { kind: "no-access" }
  | { kind: "input-form" }
  | { kind: "system" }
  | { kind: "system-grants" }
  | { kind: "system-principals" }
  | { kind: "system-agent-providers" }
  | { kind: "system-agent-provider-new" }
  | { kind: "system-agent-provider"; providerID: string }
  | { kind: "system-agent-models" }
  | { kind: "system-agent-model-new" }
  | { kind: "system-agent-model"; modelID: string }
  | { kind: "system-storage-providers" }
  | { kind: "system-storage-provider-new" }
  | { kind: "system-storage-provider"; providerID: string }
  | { kind: "system-workspace-agent-bindings" }
  | { kind: "system-workspace-agent-binding-new" }
  | {
      kind: "system-workspace-agent-binding"
      workspaceID: string
      bindingID: string
    }
  | { kind: "system-workspace-storage-bindings" }
  | { kind: "system-workspace-storage-binding-new" }
  | {
      kind: "system-workspace-storage-binding"
      workspaceID: string
      providerID: string
    }
  | ({ kind: "workspace" } & WorkspaceRoute)
  | ({ kind: "session-collection"; search: string } & WorkspaceRoute)
  | ({ kind: "project-collection"; search: string } & WorkspaceRoute)
  | ({ kind: "group-collection" } & WorkspaceRoute)
  | ({ kind: "session-chat"; mode: ChatMode; agent: string | null } & SessionRoute)
  | ({ kind: "session-files" } & SessionRoute)
  | ({ kind: "session-notes" } & SessionRoute)
  | ({ kind: "session-note-new" } & SessionRoute)
  | ({ kind: "session-note"; noteID: string } & SessionRoute)
  | ({ kind: "session-note-edit"; noteID: string } & SessionRoute)
  | ({
      kind: "session-note-revision"
      noteID: string
      revision: number
    } & SessionRoute)
  | ({ kind: "session-tasks" } & SessionRoute)
  | ({ kind: "session-task-new" } & SessionRoute)
  | ({ kind: "session-task"; taskID: string } & SessionRoute)
  | ({ kind: "session-secrets" } & SessionRoute)
  | ({ kind: "session-secret-new" } & SessionRoute)
  | ({ kind: "session-secret"; secretID: string } & SessionRoute)
  | ({ kind: "project" } & ProjectRoute)
  | ({ kind: "project-files" } & ProjectRoute)
  | ({ kind: "project-file"; fileID: string } & ProjectRoute)
  | ({ kind: "project-notes" } & ProjectRoute)
  | ({ kind: "project-note-new" } & ProjectRoute)
  | ({ kind: "project-note"; noteID: string } & ProjectRoute)
  | ({ kind: "project-tasks" } & ProjectRoute)
  | ({ kind: "project-task-new" } & ProjectRoute)
  | ({ kind: "project-task"; taskID: string } & ProjectRoute)
  | ({ kind: "project-secrets" } & ProjectRoute)
  | ({ kind: "project-secret-new" } & ProjectRoute)
  | ({ kind: "project-secret"; secretID: string } & ProjectRoute)
  | ({ kind: "project-records" } & ProjectRoute)
  | ({ kind: "project-record-schema-new" } & ProjectRoute)
  | ({ kind: "project-record-schema"; schemaID: string } & ProjectRoute)
  | ({ kind: "project-record-schema-edit"; schemaID: string } & ProjectRoute)
  | ({ kind: "project-record-new"; schemaID: string } & ProjectRoute)
  | ({
      kind: "project-record"
      schemaID: string
      recordID: string
    } & ProjectRoute)
  | ({
      kind: "project-record-edit"
      schemaID: string
      recordID: string
    } & ProjectRoute)
  | { kind: "not-found" }

export type NavigableRoute = Exclude<Route, { kind: "not-found" }>

function notFound(): Route {
  return { kind: "not-found" }
}

function pathSegments(pathname: string): string[] | null {
  const trimmed =
    pathname.length > 1 && pathname.endsWith("/")
      ? pathname.slice(0, -1)
      : pathname
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

function chatDelivery(url: URL): { mode: ChatMode; agent: string | null } {
  if (url.searchParams.get("mode") !== "direct")
    return { mode: "group", agent: null }
  return { mode: "direct", agent: url.searchParams.get("agent") }
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
  if (segments.length === 2 && segments[1] === "input") {
    return { kind: "input-form" }
  }
  if (segments.length === 2 && segments[1] === "system") {
    return { kind: "system" }
  }
  if (
    segments.length === 3 &&
    segments[1] === "system" &&
    segments[2] === "grants"
  ) {
    return { kind: "system-grants" }
  }
  if (
    segments.length === 3 &&
    segments[1] === "system" &&
    segments[2] === "principals"
  ) {
    return { kind: "system-principals" }
  }
  if (
    segments.length === 3 &&
    segments[1] === "system" &&
    segments[2] === "agent-providers"
  )
    return { kind: "system-agent-providers" }
  if (
    segments.length === 4 &&
    segments[1] === "system" &&
    segments[2] === "agent-providers" &&
    segments[3] === "new"
  )
    return { kind: "system-agent-provider-new" }
  if (
    segments.length === 4 &&
    segments[1] === "system" &&
    segments[2] === "agent-providers"
  )
    return { kind: "system-agent-provider", providerID: segments[3] }
  if (
    segments.length === 3 &&
    segments[1] === "system" &&
    segments[2] === "agent-models"
  )
    return { kind: "system-agent-models" }
  if (
    segments.length === 4 &&
    segments[1] === "system" &&
    segments[2] === "agent-models" &&
    segments[3] === "new"
  )
    return { kind: "system-agent-model-new" }
  if (
    segments.length === 4 &&
    segments[1] === "system" &&
    segments[2] === "agent-models"
  )
    return { kind: "system-agent-model", modelID: segments[3] }
  if (
    segments.length === 3 &&
    segments[1] === "system" &&
    segments[2] === "storage-providers"
  )
    return { kind: "system-storage-providers" }
  if (
    segments.length === 4 &&
    segments[1] === "system" &&
    segments[2] === "storage-providers" &&
    segments[3] === "new"
  )
    return { kind: "system-storage-provider-new" }
  if (
    segments.length === 4 &&
    segments[1] === "system" &&
    segments[2] === "storage-providers"
  )
    return { kind: "system-storage-provider", providerID: segments[3] }
  if (
    segments.length === 3 &&
    segments[1] === "system" &&
    segments[2] === "workspace-agent-bindings"
  )
    return { kind: "system-workspace-agent-bindings" }
  if (
    segments.length === 4 &&
    segments[1] === "system" &&
    segments[2] === "workspace-agent-bindings" &&
    segments[3] === "new"
  )
    return { kind: "system-workspace-agent-binding-new" }
  if (
    segments.length === 5 &&
    segments[1] === "system" &&
    segments[2] === "workspace-agent-bindings"
  )
    return {
      kind: "system-workspace-agent-binding",
      workspaceID: segments[3],
      bindingID: segments[4],
    }
  if (
    segments.length === 3 &&
    segments[1] === "system" &&
    segments[2] === "workspace-storage-bindings"
  )
    return { kind: "system-workspace-storage-bindings" }
  if (
    segments.length === 4 &&
    segments[1] === "system" &&
    segments[2] === "workspace-storage-bindings" &&
    segments[3] === "new"
  )
    return { kind: "system-workspace-storage-binding-new" }
  if (
    segments.length === 5 &&
    segments[1] === "system" &&
    segments[2] === "workspace-storage-bindings"
  )
    return {
      kind: "system-workspace-storage-binding",
      workspaceID: segments[3],
      providerID: segments[4],
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
      return {
        kind: "session-collection",
        workspaceID,
        search: collectionSearch(url),
      }
    }
    if (collection === "prj") {
      return {
        kind: "project-collection",
        workspaceID,
        search: collectionSearch(url),
      }
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
      return {
        kind: "session-chat",
        workspaceID,
        sessionID: resourceID,
        ...chatDelivery(url),
      }
    }
    const section = segments[5]
    if (segments.length === 6 && section === "files") {
      return { kind: "session-files", workspaceID, sessionID: resourceID }
    }
    if (segments.length === 6 && section === "notes") {
      return { kind: "session-notes", workspaceID, sessionID: resourceID }
    }
    if (segments.length === 7 && section === "notes") {
      return segments[6] === "new"
        ? { kind: "session-note-new", workspaceID, sessionID: resourceID }
        : {
            kind: "session-note",
            workspaceID,
            sessionID: resourceID,
            noteID: segments[6],
          }
    }
    if (
      segments.length === 8 &&
      section === "notes" &&
      segments[7] === "edit" &&
      segments[6] !== "new"
    ) {
      return {
        kind: "session-note-edit",
        workspaceID,
        sessionID: resourceID,
        noteID: segments[6],
      }
    }
    if (
      segments.length === 9 &&
      section === "notes" &&
      segments[7] === "revisions" &&
      segments[6] !== "new"
    ) {
      const revision = Number(segments[8])
      if (
        Number.isSafeInteger(revision) &&
        revision > 0 &&
        String(revision) === segments[8]
      ) {
        return {
          kind: "session-note-revision",
          workspaceID,
          sessionID: resourceID,
          noteID: segments[6],
          revision,
        }
      }
    }
    if (segments.length === 6 && section === "tasks") {
      return { kind: "session-tasks", workspaceID, sessionID: resourceID }
    }
    if (segments.length === 7 && section === "tasks") {
      return segments[6] === "new"
        ? { kind: "session-task-new", workspaceID, sessionID: resourceID }
        : {
            kind: "session-task",
            workspaceID,
            sessionID: resourceID,
            taskID: segments[6],
          }
    }
    if (segments.length === 6 && section === "secrets") {
      return { kind: "session-secrets", workspaceID, sessionID: resourceID }
    }
    if (segments.length === 7 && section === "secrets") {
      return segments[6] === "new"
        ? { kind: "session-secret-new", workspaceID, sessionID: resourceID }
        : {
            kind: "session-secret",
            workspaceID,
            sessionID: resourceID,
            secretID: segments[6],
          }
    }
    return notFound()
  }

  if (collection === "prj") {
    if (segments.length === 5) {
      return { kind: "project", workspaceID, projectID: resourceID }
    }
    const section = segments[5]
    if (segments.length === 6 && section === "files") {
      return { kind: "project-files", workspaceID, projectID: resourceID }
    }
    if (segments.length === 7 && section === "files") {
      return {
        kind: "project-file",
        workspaceID,
        projectID: resourceID,
        fileID: segments[6],
      }
    }
    if (segments.length === 6 && section === "pnt") {
      return { kind: "project-notes", workspaceID, projectID: resourceID }
    }
    if (segments.length === 7 && section === "pnt") {
      return segments[6] === "new"
        ? { kind: "project-note-new", workspaceID, projectID: resourceID }
        : {
            kind: "project-note",
            workspaceID,
            projectID: resourceID,
            noteID: segments[6],
          }
    }
    if (segments.length === 6 && section === "tasks") {
      return { kind: "project-tasks", workspaceID, projectID: resourceID }
    }
    if (segments.length === 7 && section === "tasks") {
      return segments[6] === "new"
        ? { kind: "project-task-new", workspaceID, projectID: resourceID }
        : {
            kind: "project-task",
            workspaceID,
            projectID: resourceID,
            taskID: segments[6],
          }
    }
    if (segments.length === 6 && section === "secrets") {
      return { kind: "project-secrets", workspaceID, projectID: resourceID }
    }
    if (segments.length === 7 && section === "secrets") {
      return segments[6] === "new"
        ? { kind: "project-secret-new", workspaceID, projectID: resourceID }
        : {
            kind: "project-secret",
            workspaceID,
            projectID: resourceID,
            secretID: segments[6],
          }
    }
    if (segments.length === 6 && section === "records") {
      return { kind: "project-records", workspaceID, projectID: resourceID }
    }
    if (segments.length === 7 && section === "records") {
      if (segments[6] === "new")
        return {
          kind: "project-record-schema-new",
          workspaceID,
          projectID: resourceID,
        }
      return {
        kind: "project-record-schema",
        workspaceID,
        projectID: resourceID,
        schemaID: segments[6],
      }
    }
    if (segments.length === 8 && section === "records") {
      if (segments[7] === "edit" && segments[6] !== "new")
        return {
          kind: "project-record-schema-edit",
          workspaceID,
          projectID: resourceID,
          schemaID: segments[6],
        }
      if (segments[7] === "new" && segments[6] !== "new")
        return {
          kind: "project-record-new",
          workspaceID,
          projectID: resourceID,
          schemaID: segments[6],
        }
      if (segments[6] !== "new")
        return {
          kind: "project-record",
          workspaceID,
          projectID: resourceID,
          schemaID: segments[6],
          recordID: segments[7],
        }
    }
    if (
      segments.length === 9 &&
      section === "records" &&
      segments[8] === "edit" &&
      segments[6] !== "new"
    ) {
      return {
        kind: "project-record-edit",
        workspaceID,
        projectID: resourceID,
        schemaID: segments[6],
        recordID: segments[7],
      }
    }
  }

  return notFound()
}

function segment(value: string): string {
  return encodeURIComponent(value)
}

function withSearch(pathname: string, search: string): string {
  return search === ""
    ? pathname
    : `${pathname}?${new URLSearchParams({ name: search })}`
}

export function routePath(route: NavigableRoute): string {
  switch (route.kind) {
    case "app-home":
      return "/app/"
    case "login":
      return route.next === null
        ? "/app/login"
        : `/app/login?${new URLSearchParams({ next: route.next })}`
    case "no-access":
      return "/app/no-access"
    case "input-form":
      return "/app/input"
    case "system":
      return "/app/system"
    case "system-grants":
      return "/app/system/grants"
    case "system-principals":
      return "/app/system/principals"
    case "system-agent-providers":
      return "/app/system/agent-providers"
    case "system-agent-provider-new":
      return "/app/system/agent-providers/new"
    case "system-agent-provider":
      return `/app/system/agent-providers/${segment(route.providerID)}`
    case "system-agent-models":
      return "/app/system/agent-models"
    case "system-agent-model-new":
      return "/app/system/agent-models/new"
    case "system-agent-model":
      return `/app/system/agent-models/${segment(route.modelID)}`
    case "system-storage-providers":
      return "/app/system/storage-providers"
    case "system-storage-provider-new":
      return "/app/system/storage-providers/new"
    case "system-storage-provider":
      return `/app/system/storage-providers/${segment(route.providerID)}`
    case "system-workspace-agent-bindings":
      return "/app/system/workspace-agent-bindings"
    case "system-workspace-agent-binding-new":
      return "/app/system/workspace-agent-bindings/new"
    case "system-workspace-agent-binding":
      return `/app/system/workspace-agent-bindings/${segment(route.workspaceID)}/${segment(route.bindingID)}`
    case "system-workspace-storage-bindings":
      return "/app/system/workspace-storage-bindings"
    case "system-workspace-storage-binding-new":
      return "/app/system/workspace-storage-bindings/new"
    case "system-workspace-storage-binding":
      return `/app/system/workspace-storage-bindings/${segment(route.workspaceID)}/${segment(route.providerID)}`
    case "workspace":
      return `/app/wsp/${segment(route.workspaceID)}`
    case "session-collection":
      return withSearch(
        `/app/wsp/${segment(route.workspaceID)}/ses`,
        route.search,
      )
    case "project-collection":
      return withSearch(
        `/app/wsp/${segment(route.workspaceID)}/prj`,
        route.search,
      )
    case "group-collection":
      return `/app/wsp/${segment(route.workspaceID)}/grp`
    case "session-chat":
      if (route.mode !== "direct")
        return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}`
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}?${new URLSearchParams(
        route.agent === null
          ? { mode: "direct" }
          : { mode: "direct", agent: route.agent },
      )}`
    case "session-files":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/files`
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
    case "session-tasks":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/tasks`
    case "session-task-new":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/tasks/new`
    case "session-task":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/tasks/${segment(route.taskID)}`
    case "session-secrets":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/secrets`
    case "session-secret-new":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/secrets/new`
    case "session-secret":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/secrets/${segment(route.secretID)}`
    case "project":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}`
    case "project-files":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/files`
    case "project-file":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/files/${segment(route.fileID)}`
    case "project-notes":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/pnt`
    case "project-note-new":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/pnt/new`
    case "project-note":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/pnt/${segment(route.noteID)}`
    case "project-tasks":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/tasks`
    case "project-task-new":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/tasks/new`
    case "project-task":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/tasks/${segment(route.taskID)}`
    case "project-secrets":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/secrets`
    case "project-secret-new":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/secrets/new`
    case "project-secret":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/secrets/${segment(route.secretID)}`
    case "project-records":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records`
    case "project-record-schema-new":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/new`
    case "project-record-schema":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/${segment(route.schemaID)}`
    case "project-record-schema-edit":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/${segment(route.schemaID)}/edit`
    case "project-record-new":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/${segment(route.schemaID)}/new`
    case "project-record":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/${segment(route.schemaID)}/${segment(route.recordID)}`
    case "project-record-edit":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/${segment(route.schemaID)}/${segment(route.recordID)}/edit`
  }
}
