import type { ActivityClient } from "../../app/activity"
import { fetchProject } from "../../app/projects"
import {
  addProjectMember,
  fetchProjectMembers,
  updateProjectGrant,
  projectMemberName,
  type ProjectGrant,
  type ProjectMember,
  type ProjectMemberKind,
  type ProjectMemberPage,
  type ProjectRole,
} from "../../app/project-members"
import { createDirectoryController } from "../../utils/directory-controller.svelte"

export type ProjectAccessController = ReturnType<
  typeof createProjectAccessController
>

type GrantAction = {
  mode: "role" | "remove"
  grant: ProjectGrant
  name: string
  role: ProjectRole
  kind: ProjectMemberKind
}

export function createProjectAccessController({
  activity,
  onAuthenticationLost,
  onAccessLost,
}: {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onAccessLost: () => void
}) {
  let workspace = ""
  let project = ""
  let generation = 0
  let operation: AbortController | undefined
  let unsubscribe: (() => void) | undefined
  const state = $state({
    open: false,
    canManage: false,
    saving: false,
    reloadingAction: false,
    error: "",
    adding: null as ProjectMemberKind | null,
    subject: "",
    selectedName: "",
    role: "contributor" as ProjectRole,
    action: null as GrantAction | null,
  })
  const current = (value: number, signal: AbortSignal) =>
    value === generation && !signal.aborted

  function directory(kind: ProjectMemberKind, options = false) {
    return createDirectoryController<ProjectMember>({
      onAuthenticationLost,
      async fetchPage(name, cursor, _status, signal) {
        const value = generation
        const response = await fetchProjectMembers(
          workspace, project, kind, name, cursor, signal, options,
        )
        const body = response.ok
          ? (await response.json()) as ProjectMemberPage
          : undefined
        signal.throwIfAborted()
        if (!current(value, signal))
          throw new DOMException("Obsolete project", "AbortError")
        if (response.status === 404) {
          close()
          onAccessLost()
        }
        if (response.status === 403) denyManagement()
        if (body) state.canManage = body.can_manage_access
        return { response, items: body?.members, next: body?.next_cursor }
      },
    })
  }
  const people = directory("principal")
  const groups = directory("group")
  const peopleOptions = directory("principal", true)
  const groupOptions = directory("group", true)
  const directories = { principal: people, group: groups }
  const choices = { principal: peopleOptions, group: groupOptions }

  function denyManagement() {
    state.canManage = false
    // In-flight roster/option responses must not restore a stale permission.
    people.stop()
    groups.stop()
    peopleOptions.stop()
    groupOptions.stop()
  }

  function cancelAction() {
    operation?.abort()
    state.saving = false
    state.reloadingAction = false
    state.action = null
    state.adding = null
    peopleOptions.stop()
    groupOptions.stop()
    state.error = ""
  }
  function close() {
    state.open = false
    people.stop()
    groups.stop()
    cancelAction()
  }
  function stop() {
    generation++
    close()
    unsubscribe?.()
    unsubscribe = undefined
  }
  function start(workspaceID: string, projectID: string) {
    stop()
    workspace = workspaceID
    project = projectID
    for (const directory of [people, groups, peopleOptions, groupOptions])
      directory.reset()
    state.canManage = false
    const value = generation
    unsubscribe = activity.subscribe([
      { name: "project-access", topic: `${workspace}/${project}`, events: ["project.*", "project_grant.*"] },
      { name: "project-access-groups", topic: workspace, events: ["group.*", "group_member.*", "workspace_grant.*"] },
    ], async ({ signal }) => {
      if (!current(value, signal)) return
      const response = await fetchProject(workspace, project, signal)
      if (!current(value, signal)) return
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (response.status === 404) {
        close()
        onAccessLost()
        return
      }
      if (!response.ok) throw new Error("Project access could not be refreshed.")
      if (state.open && !(await reload()))
        throw new Error("Project access could not be refreshed.")
      if (state.adding && !(await choices[state.adding].reload()))
        throw new Error("Sharing options could not be refreshed.")
    })
    void activity.poll()
    return stop
  }
  async function open() {
    state.open = true
    state.error = ""
    await Promise.all([people.retry(), groups.retry()])
  }
  async function reload() {
    return (await Promise.all([people.reload(), groups.reload()])).every(Boolean)
  }
  function beginAdd(kind: ProjectMemberKind) {
    if (!state.canManage || state.saving) return
    cancelAction()
    Object.assign(state, {
      adding: kind, subject: "", selectedName: "", role: "contributor",
    })
    choices[kind].reset()
    void choices[kind].search()
  }
  function select(id: string) {
    if (!state.adding) return
    state.subject = id
    const item = choices[state.adding].state.items.find((item) => item.id === id)
    if (item) state.selectedName = projectMemberName(item)
  }
  function beginAction(
    mode: "role" | "remove",
    kind: ProjectMemberKind,
    grant: ProjectGrant,
    name: string,
  ) {
    if (!state.canManage || state.saving) return
    cancelAction()
    // Keep the opening revision and draft independent of roster refreshes.
    state.action = { mode, kind, grant: { ...grant }, name, role: grant.role }
  }
  async function reloadAction() {
    const action = state.action
    state.error = ""
    if (!action || state.saving || state.reloadingAction) return
    state.reloadingAction = true
    if (!(await directories[action.kind].reload())) {
      if (state.action === action) {
        state.reloadingAction = false
        state.error = "The grant could not be refreshed. Try again."
      }
      return
    }
    if (state.action !== action) return
    state.reloadingAction = false
    const grant = directories[action.kind].state.items
      .flatMap((item) => item.grants)
      .find((grant) => grant.id === action.grant.id)
    if (grant) state.action = { ...action, grant: { ...grant }, role: grant.role }
    else state.error =
      "This grant is no longer on this page. Close this dialog and refresh the list."
  }
  async function mutate(request: (signal: AbortSignal) => Promise<Response>) {
    if (!state.open || !state.canManage || state.saving) return false
    operation?.abort()
    const own = new AbortController()
    const value = generation
    operation = own
    state.saving = true
    state.error = ""
    try {
      const response = await request(own.signal)
      const message = response.ok ? "" : await response.text()
      if (!current(value, own.signal)) return false
      if (response.status === 401) {
        onAuthenticationLost()
        return false
      }
      if (response.status === 404) {
        // The recipient may have gone away; re-check the project separately.
        const access = await fetchProject(workspace, project, own.signal)
        if (!current(value, own.signal)) return false
        if (access.status === 404) {
          close()
          onAccessLost()
          return false
        }
        if (access.status === 401) {
          onAuthenticationLost()
          return false
        }
      }
      if (response.status === 403) {
        denyManagement()
        throw new Error("You no longer have permission to manage project access.")
      }
      if (!response.ok)
        throw new Error(message.trim() || "Project access could not be updated. Try again.")
      state.action = null
      state.adding = null
      const refreshed = await reload()
      if (!current(value, own.signal)) return false
      if (!refreshed) state.error =
        "The change was saved, but access could not be refreshed. Try again."
      void activity.poll()
      return true
    } catch (error) {
      if (current(value, own.signal)) state.error =
        error instanceof Error ? error.message : "Project access could not be updated."
      return false
    } finally {
      if (current(value, own.signal)) state.saving = false
    }
  }
  function add() {
    const { adding, subject, role } = state
    if (!adding || !subject) return Promise.resolve(false)
    return mutate((signal) =>
      addProjectMember(workspace, project, adding, subject, role, signal),
    )
  }
  function saveAction() {
    const action = state.action
    if (!action || state.reloadingAction) return Promise.resolve(false)
    return mutate((signal) => updateProjectGrant(
      workspace,
      project,
      action.grant,
      action.mode === "remove" ? { enabled: false } : { role: action.role },
      signal,
    ))
  }
  return {
    state,
    people,
    groups,
    directory: (kind: ProjectMemberKind) => directories[kind],
    options: (kind: ProjectMemberKind) => choices[kind],
    start,
    stop,
    open,
    close,
    reload,
    cancelAction,
    beginAdd,
    select,
    beginAction,
    reloadAction,
    add,
    saveAction,
  }
}
