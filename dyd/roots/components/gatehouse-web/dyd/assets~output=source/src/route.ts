type WorkspaceRoute = { workspaceID: string }
type SessionRoute = WorkspaceRoute & { sessionID: string }
type ProjectRoute = WorkspaceRoute & { projectID: string }

export type Route =
  | { kind: "app-home" }
  | { kind: "login"; next: string | null }
  | { kind: "no-access" }
  | { kind: "system" }
  | { kind: "system-grants" }
  | { kind: "system-grant-new" }
  | { kind: "system-principals" }
  | { kind: "system-principal-new" }
  | { kind: "system-principal"; principalID: string }
  | { kind: "system-principal-edit"; principalID: string }
  | { kind: "system-agent-providers" }
  | { kind: "system-agent-provider-new" }
  | { kind: "system-codex-login"; providerID?: string }
  | { kind: "system-agent-provider"; providerID: string }
  | { kind: "system-agent-provider-edit"; providerID: string }
  | { kind: "system-agent-models" }
  | { kind: "system-agent-model-new" }
  | { kind: "system-agent-model"; modelID: string }
  | { kind: "system-agent-model-edit"; modelID: string }
  | { kind: "system-storage-providers" }
  | { kind: "system-storage-provider-new" }
  | { kind: "system-storage-provider"; providerID: string }
  | { kind: "system-storage-provider-edit"; providerID: string }
  | { kind: "system-workspaces" }
  | { kind: "system-workspace-new" }
  | ({ kind: "system-workspace" } & WorkspaceRoute)
  | ({ kind: "system-workspace-edit" } & WorkspaceRoute)
  | ({ kind: "system-workspace-principals" } & WorkspaceRoute)
  | ({ kind: "system-workspace-principal-new" } & WorkspaceRoute)
  | ({ kind: "system-workspace-groups" } & WorkspaceRoute)
  | ({ kind: "system-workspace-group"; groupID: string } & WorkspaceRoute)
  | ({ kind: "system-workspace-agent-bindings" } & WorkspaceRoute)
  | ({ kind: "system-workspace-agent-binding-new" } & WorkspaceRoute)
  | ({
      kind: "system-workspace-agent-binding"
      bindingID: string
    } & WorkspaceRoute)
  | ({
      kind: "system-workspace-agent-binding-edit"
      bindingID: string
    } & WorkspaceRoute)
  | ({ kind: "system-workspace-storage-bindings" } & WorkspaceRoute)
  | ({ kind: "system-workspace-storage-binding-new" } & WorkspaceRoute)
  | ({
      kind: "system-workspace-storage-binding"
      providerID: string
    } & WorkspaceRoute)
  | ({
      kind: "system-workspace-storage-binding-edit"
      providerID: string
    } & WorkspaceRoute)
  | ({ kind: "workspace" } & WorkspaceRoute)
  | ({
      kind: "session-collection"
      search: string
      projectID?: string
    } & WorkspaceRoute)
  | ({ kind: "project-collection"; search: string } & WorkspaceRoute)
  | ({ kind: "group-collection" } & WorkspaceRoute)
  | ({ kind: "group"; groupID: string } & WorkspaceRoute)
  | ({ kind: "session-chat" } & SessionRoute)
  | ({ kind: "session-files" } & SessionRoute)
  | ({ kind: "session-notes" } & SessionRoute)
  | ({ kind: "session-note-new" } & SessionRoute)
  | ({ kind: "session-note"; noteID: string } & SessionRoute)
  | ({ kind: "session-note-edit"; noteID: string } & SessionRoute)
  | ({ kind: "session-note-history"; noteID: string } & SessionRoute)
  | ({
      kind: "session-note-revision"
      noteID: string
      revision: number
    } & SessionRoute)
  | ({ kind: "session-tasks" } & SessionRoute)
  | ({ kind: "session-task-new" } & SessionRoute)
  | ({ kind: "session-task"; taskID: string } & SessionRoute)
  | ({ kind: "session-task-edit"; taskID: string } & SessionRoute)
  | ({ kind: "session-secrets" } & SessionRoute)
  | ({ kind: "session-secret-new" } & SessionRoute)
  | ({ kind: "session-secret"; secretID: string } & SessionRoute)
  | ({ kind: "session-secret-edit"; secretID: string } & SessionRoute)
  | ({ kind: "project" } & ProjectRoute)
  | ({ kind: "project-new" } & WorkspaceRoute)
  | ({ kind: "project-edit" } & ProjectRoute)
  | ({ kind: "project-members" } & ProjectRoute)
  | ({ kind: "project-files" } & ProjectRoute)
  | ({ kind: "project-file"; fileID: string } & ProjectRoute)
  | ({ kind: "project-notes" } & ProjectRoute)
  | ({ kind: "project-note-new" } & ProjectRoute)
  | ({ kind: "project-note"; noteID: string } & ProjectRoute)
  | ({ kind: "project-note-edit"; noteID: string } & ProjectRoute)
  | ({
      kind: "project-note-revision"
      noteID: string
      revision: number
    } & ProjectRoute)
  | ({ kind: "project-tasks" } & ProjectRoute)
  | ({ kind: "project-task-new" } & ProjectRoute)
  | ({ kind: "project-task"; taskID: string } & ProjectRoute)
  | ({ kind: "project-task-edit"; taskID: string } & ProjectRoute)
  | ({ kind: "project-records" } & ProjectRoute)
  | ({ kind: "project-record-schema-new" } & ProjectRoute)
  | ({ kind: "project-record-schema"; schemaID: string } & ProjectRoute)
  | ({ kind: "project-record-schema-edit"; schemaID: string } & ProjectRoute)
  | ({
      kind: "project-record-schema-overview"
      schemaID: string
    } & ProjectRoute)
  | ({ kind: "project-record-fields"; schemaID: string } & ProjectRoute)
  | ({ kind: "project-record-field-new"; schemaID: string } & ProjectRoute)
  | ({
      kind: "project-record-field-edit"
      schemaID: string
      attributeID: string
    } & ProjectRoute)
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
  if (segments.length === 2 && segments[1] === "system") {
    return { kind: "system" }
  }
  if (
    segments.length === 3 &&
    segments[1] === "login" &&
    segments[2] === "openai-codex"
  )
    return {
      kind: "system-codex-login",
      ...(url.searchParams.get("provider")
        ? { providerID: url.searchParams.get("provider")! }
        : {}),
    }
  if (
    segments[1] === "system" &&
    segments.length === 5 &&
    segments[4] === "edit"
  ) {
    if (segments[2] === "agent-providers")
      return { kind: "system-agent-provider-edit", providerID: segments[3] }
    if (segments[2] === "agent-models")
      return { kind: "system-agent-model-edit", modelID: segments[3] }
    if (segments[2] === "storage-providers")
      return { kind: "system-storage-provider-edit", providerID: segments[3] }
    if (segments[2] === "principals")
      return { kind: "system-principal-edit", principalID: segments[3] }
  }
  if (segments[1] === "system" && segments.length === 4) {
    if (segments[2] === "grants" && segments[3] === "new")
      return { kind: "system-grant-new" }
    if (segments[2] === "principals")
      return segments[3] === "new"
        ? { kind: "system-principal-new" }
        : { kind: "system-principal", principalID: segments[3] }
  }
  if (
    segments.length === 4 &&
    segments[1] === "system" &&
    segments[2] === "workspaces" &&
    segments[3] === "new"
  )
    return { kind: "system-workspace-new" }
  if (
    segments.length === 4 &&
    segments[1] === "system" &&
    segments[2] === "workspaces"
  )
    return { kind: "system-workspace", workspaceID: segments[3] }
  if (
    (segments.length === 5 || segments.length === 6) &&
    segments[1] === "system" &&
    segments[2] === "workspaces" &&
    segments[4] === "groups"
  )
    return segments.length === 5
      ? { kind: "system-workspace-groups", workspaceID: segments[3] }
      : {
          kind: "system-workspace-group",
          workspaceID: segments[3],
          groupID: segments[5],
        }
  if (
    segments.length === 5 &&
    segments[1] === "system" &&
    segments[2] === "workspaces" &&
    segments[4] === "edit"
  )
    return { kind: "system-workspace-edit", workspaceID: segments[3] }
  if (
    segments.length === 5 &&
    segments[1] === "system" &&
    segments[2] === "workspaces" &&
    segments[4] === "principals"
  )
    return { kind: "system-workspace-principals", workspaceID: segments[3] }
  if (
    segments.length === 6 &&
    segments[1] === "system" &&
    segments[2] === "workspaces" &&
    segments[4] === "principals" &&
    segments[5] === "new"
  )
    return { kind: "system-workspace-principal-new", workspaceID: segments[3] }
  if (
    segments.length === 5 &&
    segments[1] === "system" &&
    segments[2] === "workspaces" &&
    segments[4] === "agent-bindings"
  )
    return {
      kind: "system-workspace-agent-bindings",
      workspaceID: segments[3],
    }
  if (
    segments.length === 6 &&
    segments[1] === "system" &&
    segments[2] === "workspaces" &&
    segments[4] === "agent-bindings" &&
    segments[5] === "new"
  )
    return {
      kind: "system-workspace-agent-binding-new",
      workspaceID: segments[3],
    }
  if (
    segments.length === 6 &&
    segments[1] === "system" &&
    segments[2] === "workspaces" &&
    segments[4] === "agent-bindings"
  )
    return {
      kind: "system-workspace-agent-binding",
      workspaceID: segments[3],
      bindingID: segments[5],
    }
  if (
    segments.length === 7 &&
    segments[1] === "system" &&
    segments[2] === "workspaces" &&
    segments[4] === "agent-bindings" &&
    segments[6] === "edit"
  )
    return {
      kind: "system-workspace-agent-binding-edit",
      workspaceID: segments[3],
      bindingID: segments[5],
    }
  if (
    segments.length === 5 &&
    segments[1] === "system" &&
    segments[2] === "workspaces" &&
    segments[4] === "storage-bindings"
  )
    return {
      kind: "system-workspace-storage-bindings",
      workspaceID: segments[3],
    }
  if (
    segments.length === 6 &&
    segments[1] === "system" &&
    segments[2] === "workspaces" &&
    segments[4] === "storage-bindings" &&
    segments[5] === "new"
  )
    return {
      kind: "system-workspace-storage-binding-new",
      workspaceID: segments[3],
    }
  if (
    segments.length === 6 &&
    segments[1] === "system" &&
    segments[2] === "workspaces" &&
    segments[4] === "storage-bindings"
  )
    return {
      kind: "system-workspace-storage-binding",
      workspaceID: segments[3],
      providerID: segments[5],
    }
  if (
    segments.length === 7 &&
    segments[1] === "system" &&
    segments[2] === "workspaces" &&
    segments[4] === "storage-bindings" &&
    segments[6] === "edit"
  )
    return {
      kind: "system-workspace-storage-binding-edit",
      workspaceID: segments[3],
      providerID: segments[5],
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
    segments[2] === "workspaces"
  )
    return { kind: "system-workspaces" }
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
        ...(url.searchParams.get("project")
          ? { projectID: url.searchParams.get("project")! }
          : {}),
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
  if (collection === "grp" && segments.length === 5)
    return { kind: "group", workspaceID, groupID: resourceID }
  if (collection === "ses") {
    if (segments.length === 5) {
      return {
        kind: "session-chat",
        workspaceID,
        sessionID: resourceID,
      }
    }
    const section = segments[5]
    if (
      segments.length === 8 &&
      segments[7] === "edit" &&
      segments[6] !== "new"
    ) {
      if (section === "tasks")
        return {
          kind: "session-task-edit",
          workspaceID,
          sessionID: resourceID,
          taskID: segments[6],
        }
      if (section === "secrets")
        return {
          kind: "session-secret-edit",
          workspaceID,
          sessionID: resourceID,
          secretID: segments[6],
        }
    }
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
      segments.length === 8 &&
      section === "notes" &&
      segments[7] === "revisions" &&
      segments[6] !== "new"
    ) {
      return {
        kind: "session-note-history",
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
    if (segments.length === 5 && resourceID === "new")
      return { kind: "project-new", workspaceID }
    if (segments.length === 6 && segments[5] === "edit" && resourceID !== "new")
      return { kind: "project-edit", workspaceID, projectID: resourceID }
    if (segments.length === 5) {
      return { kind: "project", workspaceID, projectID: resourceID }
    }
    const section = segments[5]
    if (
      segments.length === 8 &&
      segments[7] === "edit" &&
      segments[6] !== "new"
    ) {
      if (section === "pnt")
        return {
          kind: "project-note-edit",
          workspaceID,
          projectID: resourceID,
          noteID: segments[6],
        }
      if (section === "tasks")
        return {
          kind: "project-task-edit",
          workspaceID,
          projectID: resourceID,
          taskID: segments[6],
        }
    }
    if (
      segments.length === 9 &&
      section === "pnt" &&
      segments[7] === "revisions" &&
      segments[6] !== "new"
    ) {
      const revision = Number(segments[8])
      if (
        Number.isSafeInteger(revision) &&
        revision > 0 &&
        String(revision) === segments[8]
      )
        return {
          kind: "project-note-revision",
          workspaceID,
          projectID: resourceID,
          noteID: segments[6],
          revision,
        }
      return notFound()
    }
    if (segments.length === 6 && section === "files") {
      return { kind: "project-files", workspaceID, projectID: resourceID }
    }
    if (segments.length === 6 && section === "members") {
      return { kind: "project-members", workspaceID, projectID: resourceID }
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
      if (segments[6] !== "new" && segments[7] === "overview")
        return {
          kind: "project-record-schema-overview",
          workspaceID,
          projectID: resourceID,
          schemaID: segments[6],
        }
      if (segments[6] !== "new" && segments[7] === "fields")
        return {
          kind: "project-record-fields",
          workspaceID,
          projectID: resourceID,
          schemaID: segments[6],
        }
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
      section === "records" &&
      segments[6] !== "new" &&
      segments[7] === "fields"
    ) {
      if (segments.length === 9 && segments[8] === "new")
        return {
          kind: "project-record-field-new",
          workspaceID,
          projectID: resourceID,
          schemaID: segments[6],
        }
      if (
        segments.length === 10 &&
        segments[8] !== "new" &&
        segments[9] === "edit"
      )
        return {
          kind: "project-record-field-edit",
          workspaceID,
          projectID: resourceID,
          schemaID: segments[6],
          attributeID: segments[8],
        }
      return notFound()
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

export function legacySystemRedirect(url: URL): string | null {
  const segments = pathSegments(url.pathname)
  if (segments === null || segments[0] !== "app" || segments[1] !== "system")
    return null
  if (segments[2] === "workspace-agent-bindings") {
    if (segments.length === 5)
      return `/app/system/workspaces/${segment(segments[3])}/agent-bindings/${segment(segments[4])}`
    return "/app/system/workspaces"
  }
  if (segments[2] === "workspace-storage-bindings") {
    if (segments.length === 5)
      return `/app/system/workspaces/${segment(segments[3])}/storage-bindings/${segment(segments[4])}`
    return "/app/system/workspaces"
  }
  return null
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
    case "system":
      return "/app/system"
    case "system-grants":
      return "/app/system/grants"
    case "system-grant-new":
      return "/app/system/grants/new"
    case "system-principals":
      return "/app/system/principals"
    case "system-principal-new":
      return "/app/system/principals/new"
    case "system-principal":
      return `/app/system/principals/${segment(route.principalID)}`
    case "system-principal-edit":
      return `/app/system/principals/${segment(route.principalID)}/edit`
    case "system-agent-providers":
      return "/app/system/agent-providers"
    case "system-agent-provider-new":
      return "/app/system/agent-providers/new"
    case "system-codex-login":
      return (
        "/app/login/openai-codex" +
        (route.providerID
          ? `?${new URLSearchParams({ provider: route.providerID })}`
          : "")
      )
    case "system-agent-provider":
      return `/app/system/agent-providers/${segment(route.providerID)}`
    case "system-agent-provider-edit":
      return `/app/system/agent-providers/${segment(route.providerID)}/edit`
    case "system-agent-models":
      return "/app/system/agent-models"
    case "system-agent-model-new":
      return "/app/system/agent-models/new"
    case "system-agent-model":
      return `/app/system/agent-models/${segment(route.modelID)}`
    case "system-agent-model-edit":
      return `/app/system/agent-models/${segment(route.modelID)}/edit`
    case "system-storage-providers":
      return "/app/system/storage-providers"
    case "system-storage-provider-new":
      return "/app/system/storage-providers/new"
    case "system-storage-provider":
      return `/app/system/storage-providers/${segment(route.providerID)}`
    case "system-storage-provider-edit":
      return `/app/system/storage-providers/${segment(route.providerID)}/edit`
    case "system-workspaces":
      return "/app/system/workspaces"
    case "system-workspace-new":
      return "/app/system/workspaces/new"
    case "system-workspace":
      return `/app/system/workspaces/${segment(route.workspaceID)}`
    case "system-workspace-edit":
      return `/app/system/workspaces/${segment(route.workspaceID)}/edit`
    case "system-workspace-principals":
      return `/app/system/workspaces/${segment(route.workspaceID)}/principals`
    case "system-workspace-principal-new":
      return `/app/system/workspaces/${segment(route.workspaceID)}/principals/new`
    case "system-workspace-groups":
      return `/app/system/workspaces/${segment(route.workspaceID)}/groups`
    case "system-workspace-group":
      return `/app/system/workspaces/${segment(route.workspaceID)}/groups/${segment(route.groupID)}`
    case "system-workspace-agent-bindings":
      return `/app/system/workspaces/${segment(route.workspaceID)}/agent-bindings`
    case "system-workspace-agent-binding-new":
      return `/app/system/workspaces/${segment(route.workspaceID)}/agent-bindings/new`
    case "system-workspace-agent-binding":
      return `/app/system/workspaces/${segment(route.workspaceID)}/agent-bindings/${segment(route.bindingID)}`
    case "system-workspace-agent-binding-edit":
      return `/app/system/workspaces/${segment(route.workspaceID)}/agent-bindings/${segment(route.bindingID)}/edit`
    case "system-workspace-storage-bindings":
      return `/app/system/workspaces/${segment(route.workspaceID)}/storage-bindings`
    case "system-workspace-storage-binding-new":
      return `/app/system/workspaces/${segment(route.workspaceID)}/storage-bindings/new`
    case "system-workspace-storage-binding":
      return `/app/system/workspaces/${segment(route.workspaceID)}/storage-bindings/${segment(route.providerID)}`
    case "system-workspace-storage-binding-edit":
      return `/app/system/workspaces/${segment(route.workspaceID)}/storage-bindings/${segment(route.providerID)}/edit`
    case "workspace":
      return `/app/wsp/${segment(route.workspaceID)}`
    case "session-collection":
      return route.projectID
        ? `/app/wsp/${segment(route.workspaceID)}/ses?${new URLSearchParams({ ...(route.search ? { name: route.search } : {}), project: route.projectID })}`
        : withSearch(`/app/wsp/${segment(route.workspaceID)}/ses`, route.search)
    case "project-collection":
      return withSearch(
        `/app/wsp/${segment(route.workspaceID)}/prj`,
        route.search,
      )
    case "group-collection":
      return `/app/wsp/${segment(route.workspaceID)}/grp`
    case "group":
      return `/app/wsp/${segment(route.workspaceID)}/grp/${segment(route.groupID)}`
    case "session-chat":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}`
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
    case "session-note-history":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/notes/${segment(route.noteID)}/revisions`
    case "session-note-revision":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/notes/${segment(route.noteID)}/revisions/${segment(String(route.revision))}`
    case "session-tasks":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/tasks`
    case "session-task-new":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/tasks/new`
    case "session-task":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/tasks/${segment(route.taskID)}`
    case "session-task-edit":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/tasks/${segment(route.taskID)}/edit`
    case "session-secrets":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/secrets`
    case "session-secret-new":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/secrets/new`
    case "session-secret":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/secrets/${segment(route.secretID)}`
    case "session-secret-edit":
      return `/app/wsp/${segment(route.workspaceID)}/ses/${segment(route.sessionID)}/secrets/${segment(route.secretID)}/edit`
    case "project":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}`
    case "project-new":
      return `/app/wsp/${segment(route.workspaceID)}/prj/new`
    case "project-edit":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/edit`
    case "project-members":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/members`
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
    case "project-note-edit":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/pnt/${segment(route.noteID)}/edit`
    case "project-note-revision":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/pnt/${segment(route.noteID)}/revisions/${segment(String(route.revision))}`
    case "project-tasks":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/tasks`
    case "project-task-new":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/tasks/new`
    case "project-task":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/tasks/${segment(route.taskID)}`
    case "project-task-edit":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/tasks/${segment(route.taskID)}/edit`
    case "project-records":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records`
    case "project-record-schema-new":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/new`
    case "project-record-schema":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/${segment(route.schemaID)}`
    case "project-record-schema-edit":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/${segment(route.schemaID)}/edit`
    case "project-record-schema-overview":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/${segment(route.schemaID)}/overview`
    case "project-record-fields":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/${segment(route.schemaID)}/fields`
    case "project-record-field-new":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/${segment(route.schemaID)}/fields/new`
    case "project-record-field-edit":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/${segment(route.schemaID)}/fields/${segment(route.attributeID)}/edit`
    case "project-record-new":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/${segment(route.schemaID)}/new`
    case "project-record":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/${segment(route.schemaID)}/${segment(route.recordID)}`
    case "project-record-edit":
      return `/app/wsp/${segment(route.workspaceID)}/prj/${segment(route.projectID)}/records/${segment(route.schemaID)}/${segment(route.recordID)}/edit`
  }
}
