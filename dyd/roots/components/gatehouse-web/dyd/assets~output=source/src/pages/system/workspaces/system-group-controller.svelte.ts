import {
  groupAdminPath,
  type SystemGroup,
  type SystemGroupGrant,
  type SystemGroupMember,
  type SystemGroupAgent,
} from "../../../app/group-administration"
import { systemAdministration, type WorkspaceRole } from "../../../app/system"
import { createLoadState } from "../../../utils/load-state.svelte"

type FeedbackScope = "details" | "members" | "bots" | "grants"
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
    mutation = createLoadState(),
    grantsResource = createLoadState()
  let disposed = false
  const state = $state({
    group: null as SystemGroup | null,
    grants: [] as SystemGroupGrant[],
    form: { alias: "", name: "", enabled: true, revision: 0 },
    principal: "",
    agent: "",
    newRole: "member" as WorkspaceRole,
    editingGrant: null as SystemGroupGrant | null,
    grantRole: "member" as WorkspaceRole,
    grantEnabled: true,
    groupConflict: false,
    grantConflict: false,
    error: "",
    notice: "",
    grantsLoadError: "",
    pendingAction: "",
    feedback: {
      details: { error: "", notice: "" },
      members: { error: "", notice: "" },
      bots: { error: "", notice: "" },
      grants: { error: "", notice: "" },
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
    const replaceDraft = resetDraft || state.group === null
    state.group = group
    if (replaceDraft) {
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
      const path = groupAdminPath(workspaceID(), groupID())
      const [groupResponse, grantsResponse] = await Promise.all([
        systemAdministration(path, "GET", undefined, request.signal),
        systemAdministration(
          `${path}/grants`,
          "GET",
          undefined,
          request.signal,
        ),
      ])
      if (!request.current() || denied(groupResponse) || denied(grantsResponse))
        return
      if (!groupResponse.ok || !grantsResponse.ok)
        throw new Error("Group could not be loaded. Try again.")
      const [group, grants] = await Promise.all([
        groupResponse.json() as Promise<SystemGroup>,
        grantsResponse.json() as Promise<{ grants: SystemGroupGrant[] }>,
      ])
      request.commit(() => {
        acceptGroup(group, resetDraft)
        state.grants = grants.grants
      })
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
    }: {
      scope?: FeedbackScope
      action?: string
      notice?: string
    } = {},
  ): Promise<{ ok: boolean; conflict: boolean }> {
    if (disposed || state.saving) return { ok: false, conflict: false }
    const request = mutation.begin()
    state.pendingAction = action
    clearFeedback(scope)
    state.error = state.notice = ""
    const fail = (message: string) => {
      state.error = state.feedback[scope].error = message
    }
    let conflict = false
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
        conflict =
          response.status === 409 && message.includes("changed elsewhere")
        fail(message.trim() || "Change could not be saved.")
        return { ok: false, conflict }
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
    return { ok: false, conflict }
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
  async function addMember() {
    if (!state.principal) {
      state.error = state.feedback.members.error = "Choose a person."
      return false
    }
    const principal = state.principal
    const result = await mutate(
      groupAdminPath(workspaceID(), groupID(), "/members"),
      "POST",
      { principal },
      async (_response, current) => {
        if (current()) {
          if (state.principal === principal) state.principal = ""
          if ((await onMembersChanged()) === false && current())
            state.feedback.members.error =
              "Membership saved, but the list could not be refreshed. Reload the list."
        }
      },
      { scope: "members", action: "members:add", notice: "Member added." },
    )
    return result.ok
  }
  async function toggleMember(member: SystemGroupMember) {
    const result = await mutate(
      groupAdminPath(
        workspaceID(),
        groupID(),
        `/members/${encodeURIComponent(member.principal)}`,
      ),
      "PATCH",
      { enabled: !member.enabled, expected_revision: member.revision },
      async (_response, current) => {
        if ((await onMembersChanged()) === false && current())
          state.feedback.members.error =
            "Membership saved, but the list could not be refreshed. Reload the list."
      },
      {
        scope: "members",
        action: `member:${member.principal}`,
        notice: member.enabled ? "Membership removed." : "Membership enabled.",
      },
    )
    if (result.conflict && !disposed) await onMembersChanged()
  }
  async function addAgent() {
    if (!state.agent) {
      state.error = state.feedback.bots.error = "Choose a bot."
      return false
    }
    const agent = state.agent
    const result = await mutate(
      groupAdminPath(workspaceID(), groupID(), "/agents"),
      "POST",
      { agent },
      async (_response, current) => {
        if (current()) {
          if (state.agent === agent) state.agent = ""
          if ((await onAgentsChanged()) === false && current())
            state.feedback.bots.error =
              "Membership saved, but the list could not be refreshed. Reload the list."
        }
      },
      { scope: "bots", action: "bots:add", notice: "Bot added." },
    )
    return result.ok
  }
  async function toggleAgent(agent: SystemGroupAgent) {
    const result = await mutate(
      groupAdminPath(
        workspaceID(),
        groupID(),
        `/agents/${encodeURIComponent(agent.agent)}`,
      ),
      "PATCH",
      { enabled: !agent.enabled, expected_revision: agent.revision },
      async (_response, current) => {
        if ((await onAgentsChanged()) === false && current())
          state.feedback.bots.error =
            "Membership saved, but the list could not be refreshed. Reload the list."
      },
      {
        scope: "bots",
        action: `bot:${agent.agent}`,
        notice: agent.enabled
          ? "Bot membership removed."
          : "Bot membership enabled.",
      },
    )
    if (result.conflict && !disposed) await onAgentsChanged()
  }
  async function createGrant() {
    const result = await mutate(
      groupAdminPath(workspaceID(), groupID(), "/grants"),
      "POST",
      { role: state.newRole },
      async () => {
        await refreshGrants()
      },
      {
        scope: "grants",
        action: "grants:add",
        notice: "Workspace grant added.",
      },
    )
    return result.ok
  }
  async function refreshGrants() {
    if (disposed) return
    const request = grantsResource.begin()
    try {
      const response = await systemAdministration(
        groupAdminPath(workspaceID(), groupID(), "/grants"),
        "GET",
        undefined,
        request.signal,
      )
      if (!request.current() || denied(response)) return
      if (!response.ok)
        throw new Error("Workspace access could not be refreshed.")
      const body = (await response.json()) as { grants: SystemGroupGrant[] }
      request.commit(() => {
        state.grants = body.grants
        state.grantsLoadError = ""
      })
    } catch {
      if (request.current())
        state.grantsLoadError =
          "Workspace access could not be refreshed. Reload the list."
    } finally {
      request.finish()
    }
  }
  function beginGrant(grant: SystemGroupGrant) {
    if (disposed || state.saving) return
    state.editingGrant = { ...grant }
    state.grantRole = grant.role
    state.grantEnabled = grant.enabled
    state.grantConflict = false
    state.error = state.notice = ""
    clearFeedback("grants")
  }
  function cancelGrant() {
    if (!state.saving) state.editingGrant = null
  }
  async function saveGrant() {
    if (!state.editingGrant || state.grantConflict) return false
    const grant = state.editingGrant
    const result = await mutate(
      `workspaces/${encodeURIComponent(workspaceID())}/grants/${encodeURIComponent(grant.id)}`,
      "PATCH",
      {
        role: state.grantRole,
        enabled: state.grantEnabled,
        expected_revision: grant.revision,
      },
      async (_response, current) => {
        if (current()) state.editingGrant = null
        await refreshGrants()
      },
      {
        scope: "grants",
        action: `grant:${grant.id}`,
        notice: "Workspace grant saved.",
      },
    )
    if (result.conflict && !disposed) {
      state.grantConflict = true
      state.error = state.feedback.grants.error =
        "This grant changed elsewhere. Cancel and reopen it to use the latest revision."
      await refreshGrants()
    }
    return result.ok
  }
  async function toggleGrant(grant: SystemGroupGrant) {
    const result = await mutate(
      `workspaces/${encodeURIComponent(workspaceID())}/grants/${encodeURIComponent(grant.id)}`,
      "PATCH",
      { enabled: !grant.enabled, expected_revision: grant.revision },
      async (_response, current) => {
        if (current() && state.editingGrant?.id === grant.id)
          state.editingGrant = null
        await refreshGrants()
      },
      {
        scope: "grants",
        action: `grant:${grant.id}`,
        notice: grant.enabled
          ? "Workspace grant revoked."
          : "Workspace grant enabled.",
      },
    )
    if (result.conflict && !disposed) await refreshGrants()
  }
  function stop() {
    disposed = true
    resource.stop()
    mutation.stop()
    grantsResource.stop()
    state.pendingAction = ""
  }
  return {
    state,
    clearFeedback,
    beginDetails,
    refreshGrants,
    load,
    create,
    saveGroup,
    addMember,
    toggleMember,
    addAgent,
    toggleAgent,
    createGrant,
    beginGrant,
    cancelGrant,
    saveGrant,
    toggleGrant,
    stop,
  }
}
