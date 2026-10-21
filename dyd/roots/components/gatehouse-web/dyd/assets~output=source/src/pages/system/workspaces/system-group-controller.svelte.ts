import {
  groupAdminPath,
  groupAdministration,
  type GroupScope,
  type GroupRole,
  type SystemGroup,
  type SystemGroupMember,
  type SystemGroupAgent,
} from "../../../app/group-administration"
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
  scope = "system",
}: {
  workspaceID: () => string
  groupID: () => string
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
  onMembersChanged?: () => Promise<unknown>
  onAgentsChanged?: () => Promise<unknown>
  scope?: GroupScope
}) {
  const resource = createLoadState(),
    mutation = createLoadState()
  let disposed = false
  const state = $state({
    group: null as SystemGroup | null,
    form: { alias: "", name: "", enabled: true, revision: 0 },
    principal: "",
    memberRole: "member" as GroupRole,
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
          state.form.name !== (state.group.name ?? ""))
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
    if (response.status === 403 && scope === "system") {
      onSystemAccessChange("denied")
      return true
    }
    return false
  }
  async function load(
    resetDraft = true,
    signal?: AbortSignal,
  ): Promise<boolean> {
    if (disposed) return false
    const request = resource.begin(signal)
    try {
      const response = await groupAdministration(
        groupAdminPath(workspaceID(), groupID()),
        "GET",
        undefined,
        request.signal,
        scope,
      )
      if (!request.current()) return false
      if ([401, 403, 404].includes(response.status)) {
        request.commit(() => {
          state.group = null
        })
        request.fail(
          new Error(
            response.status === 401
              ? "Sign in is required."
              : "This group is no longer available to you.",
          ),
        )
        denied(response)
        return true
      }
      if (!response.ok) throw new Error("Group could not be loaded. Try again.")
      const group = (await response.json()) as SystemGroup
      return request.commit(() => acceptGroup(group, resetDraft))
    } catch (error) {
      request.fail(error)
      return false
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
      scope: feedbackScope = "details",
      action = feedbackScope,
      notice = "Changes saved.",
    }: { scope?: FeedbackScope; action?: string; notice?: string } = {},
  ) {
    if (disposed || state.saving) return { ok: false, conflict: false }
    const request = mutation.begin()
    state.pendingAction = action
    clearFeedback(feedbackScope)
    state.error = state.notice = ""
    const fail = (message: string) => {
      state.error = state.feedback[feedbackScope].error = message
    }
    try {
      const response = await groupAdministration(
        path,
        method,
        body,
        request.signal,
        scope,
      )
      if (!request.current() || denied(response))
        return { ok: false, conflict: false }
      if (!response.ok) {
        const message = await response.text()
        if (!request.current()) return { ok: false, conflict: false }
        fail(message.trim() || "Change could not be saved.")
        if (
          scope === "workspace" &&
          (response.status === 403 || response.status === 404)
        )
          await load(false)
        return {
          ok: false,
          conflict:
            response.status === 409 && message.includes("changed elsewhere"),
        }
      }
      await commit(response, request.current)
      if (!request.current()) return { ok: false, conflict: false }
      if (!state.feedback[feedbackScope].error)
        state.notice = state.feedback[feedbackScope].notice = notice
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
  async function deleteGroup() {
    if (!state.group) return false
    const result = await mutate(
      groupAdminPath(workspaceID(), groupID()),
      "PATCH",
      { enabled: false, expected_revision: state.group.revision },
      async (_response, current) => {
        if (current()) state.group = null
      },
      { action: "details:delete", notice: "Group deleted." },
    )
    if (result.conflict && !disposed) await load(false)
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
      {
        [kind === "members" ? "principal" : "agent"]: subject,
        ...(kind === "members" ? { role: state.memberRole } : {}),
      },
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
        if (current() && scope === "workspace") await load(false)
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
  async function changeMemberRole(member: SystemGroupMember, role: GroupRole) {
    const result = await mutate(
      groupAdminPath(
        workspaceID(),
        groupID(),
        `/members/${encodeURIComponent(member.principal)}`,
      ),
      "PATCH",
      { role, expected_revision: member.revision },
      async (_response, current) => {
        if ((await onMembersChanged()) === false && current())
          state.feedback.members.error =
            "Role saved, but the list could not be refreshed. Reload the list."
        if (current() && scope === "workspace") await load(false)
      },
      {
        scope: "members",
        action: `member:${member.principal}`,
        notice: "Member role updated.",
      },
    )
    if (result.conflict && !disposed) await onMembersChanged()
    return result.ok
  }
  return {
    state,
    clearFeedback,
    beginDetails,
    load,
    create,
    saveGroup,
    deleteGroup,
    addMember: () => addMembership("members"),
    addAgent: () => addMembership("bots"),
    changeMemberRole,
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
