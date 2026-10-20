import {
  groupAdminPath,
  type SystemGroup,
  type SystemGroupMember,
  type SystemGroupAgent,
} from "../../../app/group-administration"
import { systemAdministration } from "../../../app/system"
import { createLoadState } from "../../../utils/load-state.svelte"

type FeedbackScope = "details" | "members" | "bots"
export type SystemGroupController = ReturnType<
  typeof createSystemGroupController
>

export function createSystemGroupController({
  workspaceID,
  groupID,
  onAuthenticationLost,
  onSystemAccessChange,
  onMembersChanged = async () => {},
  onAgentsChanged = async () => {},
}: {
  workspaceID: () => string
  groupID: () => string
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
  onMembersChanged?: () => Promise<unknown>
  onAgentsChanged?: () => Promise<unknown>
}) {
  const resource = createLoadState(),
    mutation = createLoadState()
  let disposed = false
  const state = $state({
    group: null as SystemGroup | null,
    form: { alias: "", name: "", enabled: true, revision: 0 },
    principal: "",
    agent: "",
    groupConflict: false,
    error: "",
    notice: "",
    pendingAction: "",
    feedback: {
      details: { error: "", notice: "" },
      members: { error: "", notice: "" },
      bots: { error: "", notice: "" },
    },
    get dirty(): boolean {
      return (
        state.group !== null &&
        (state.form.alias !== (state.group.alias ?? "") ||
          state.form.name !== (state.group.name ?? "") ||
          state.form.enabled !== state.group.enabled)
      )
    },
    get reloading() {
      return resource.reloading
    },
    get saving() {
      return mutation.state.pending
    },
    get loading() {
      return resource.loading
    },
    get loadError() {
      return resource.state.error
    },
  })
  function acceptGroup(group: SystemGroup, resetDraft: boolean) {
    const reset = resetDraft || state.group === null
    state.group = group
    if (reset) {
      state.form = {
        alias: group.alias ?? "",
        name: group.name ?? "",
        enabled: group.enabled,
        revision: group.revision,
      }
      state.groupConflict = false
    }
  }
  function clearFeedback(scope: FeedbackScope) {
    state.feedback[scope].error = state.feedback[scope].notice = ""
  }
  function beginDetails() {
    if (state.saving || !state.group) return
    acceptGroup(state.group, true)
    clearFeedback("details")
  }
  function denied(response: Response) {
    if (response.status === 401) {
      onAuthenticationLost()
      return true
    }
    if (response.status === 403) {
      onSystemAccessChange("denied")
      return true
    }
    return false
  }
  async function load(resetDraft = true) {
    if (disposed) return
    const request = resource.begin()
    try {
      const response = await systemAdministration(
        groupAdminPath(workspaceID(), groupID()),
        "GET",
        undefined,
        request.signal,
      )
      if (!request.current() || denied(response)) return
      if (!response.ok) throw new Error("Group could not be loaded. Try again.")
      const group = (await response.json()) as SystemGroup
      request.commit(() => acceptGroup(group, resetDraft))
    } catch (error) {
      request.fail(error)
    } finally {
      request.finish()
    }
  }
  async function mutate(
    path: string,
    method: "POST" | "PATCH",
    body: Record<string, unknown>,
    commit: (response: Response, current: () => boolean) => Promise<void>,
    {
      scope = "details",
      action = scope,
      notice = "Changes saved.",
    }: { scope?: FeedbackScope; action?: string; notice?: string } = {},
  ) {
    if (disposed || state.saving) return { ok: false, conflict: false }
    const request = mutation.begin()
    state.pendingAction = action
    clearFeedback(scope)
    state.error = state.notice = ""
    const fail = (message: string) => {
      state.error = state.feedback[scope].error = message
    }
    try {
      const response = await systemAdministration(
        path,
        method,
        body,
        request.signal,
      )
      if (!request.current() || denied(response))
        return { ok: false, conflict: false }
      if (!response.ok) {
        const message = await response.text()
        if (!request.current()) return { ok: false, conflict: false }
        fail(message.trim() || "Change could not be saved.")
        return {
          ok: false,
          conflict:
            response.status === 409 && message.includes("changed elsewhere"),
        }
      }
      await commit(response, request.current)
      if (!request.current()) return { ok: false, conflict: false }
      if (!state.feedback[scope].error)
        state.notice = state.feedback[scope].notice = notice
      return { ok: true, conflict: false }
    } catch {
      if (request.current()) fail("Change could not be saved. Try again.")
    } finally {
      if (request.current()) state.pendingAction = ""
      request.finish()
    }
    return { ok: false, conflict: false }
  }
  async function create() {
    let created: SystemGroup | null = null
    const result = await mutate(
      groupAdminPath(workspaceID()),
      "POST",
      {
        alias: state.form.alias,
        name: state.form.name,
        enabled: state.form.enabled,
      },
      async (response, current) => {
        const group = (await response.json()) as SystemGroup
        if (current()) created = group
      },
    )
    return result.ok && !disposed ? (created as SystemGroup | null) : null
  }
  async function saveGroup() {
    if (state.groupConflict || !state.group) return false
    const result = await mutate(
      groupAdminPath(workspaceID(), groupID()),
      "PATCH",
      {
        alias: state.form.alias,
        name: state.form.name,
        enabled: state.form.enabled,
        expected_revision: state.form.revision,
      },
      async (response, current) => {
        const group = (await response.json()) as SystemGroup
        if (current()) acceptGroup(group, true)
      },
      { action: "details:save", notice: "Group details saved." },
    )
    if (result.conflict && !disposed) state.groupConflict = true
    return result.ok
  }
  async function addMembership(kind: "members" | "bots") {
    const subject = kind === "members" ? state.principal : state.agent
    if (!subject) {
      state.error = state.feedback[kind].error =
        kind === "members" ? "Choose a person." : "Choose a bot."
      return false
    }
    const result = await mutate(
      groupAdminPath(
        workspaceID(),
        groupID(),
        kind === "members" ? "/members" : "/agents",
      ),
      "POST",
      { [kind === "members" ? "principal" : "agent"]: subject },
      async (_response, current) => {
        if (!current()) return
        if (kind === "members" && state.principal === subject)
          state.principal = ""
        if (kind === "bots" && state.agent === subject) state.agent = ""
        const refreshed = await (kind === "members"
          ? onMembersChanged()
          : onAgentsChanged())
        if (refreshed === false && current())
          state.feedback[kind].error =
            "Membership saved, but the list could not be refreshed. Reload the list."
      },
      {
        scope: kind,
        action: `${kind}:add`,
        notice: kind === "members" ? "Member added." : "Bot added.",
      },
    )
    return result.ok
  }
  async function toggleMembership(
    kind: "members" | "bots",
    subject: string,
    enabled: boolean,
    revision: number,
  ) {
    const refresh = kind === "members" ? onMembersChanged : onAgentsChanged
    const result = await mutate(
      groupAdminPath(
        workspaceID(),
        groupID(),
        `/${kind === "members" ? "members" : "agents"}/${encodeURIComponent(subject)}`,
      ),
      "PATCH",
      { enabled: !enabled, expected_revision: revision },
      async (_response, current) => {
        if ((await refresh()) === false && current())
          state.feedback[kind].error =
            "Membership saved, but the list could not be refreshed. Reload the list."
      },
      {
        scope: kind,
        action: `${kind === "members" ? "member" : "bot"}:${subject}`,
        notice: enabled ? "Membership removed." : "Membership enabled.",
      },
    )
    if (result.conflict && !disposed) await refresh()
  }
  function stop() {
    disposed = true
    resource.stop()
    mutation.stop()
    state.pendingAction = ""
  }
  return {
    state,
    clearFeedback,
    beginDetails,
    load,
    create,
    saveGroup,
    addMember: () => addMembership("members"),
    addAgent: () => addMembership("bots"),
    toggleMember: (member: SystemGroupMember) =>
      toggleMembership(
        "members",
        member.principal,
        member.enabled,
        member.revision,
      ),
    toggleAgent: (agent: SystemGroupAgent) =>
      toggleMembership("bots", agent.agent, agent.enabled, agent.revision),
    stop,
  }
}
