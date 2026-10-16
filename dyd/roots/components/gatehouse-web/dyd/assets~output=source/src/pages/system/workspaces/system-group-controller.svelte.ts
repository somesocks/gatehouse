import {
  groupAdminPath,
  type SystemGroup,
  type SystemGroupGrant,
  type SystemGroupMember,
} from "../../../app/group-administration"
import { systemAdministration, type WorkspaceRole } from "../../../app/system"
import { createLoadState } from "../../../utils/load-state.svelte"

export function createSystemGroupController({
  workspaceID,
  groupID,
  onAuthenticationLost,
  onSystemAccessChange,
  onMembersChanged = async () => {},
}: {
  workspaceID: () => string
  groupID: () => string
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
  onMembersChanged?: () => Promise<unknown>
}) {
  const resource = createLoadState(),
    mutation = createLoadState()
  let disposed = false
  const state = $state({
    group: null as SystemGroup | null,
    grants: [] as SystemGroupGrant[],
    form: { alias: "", name: "", enabled: true, revision: 0 },
    principal: "",
    newRole: "member" as WorkspaceRole,
    editingGrant: null as SystemGroupGrant | null,
    grantRole: "member" as WorkspaceRole,
    grantEnabled: true,
    groupConflict: false,
    grantConflict: false,
    error: "",
    notice: "",
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
  ): Promise<{ ok: boolean; conflict: boolean }> {
    if (disposed || state.saving) return { ok: false, conflict: false }
    const request = mutation.begin()
    state.error = state.notice = ""
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
        state.error = message.trim() || "Change could not be saved."
        return { ok: false, conflict }
      }
      await commit(response, request.current)
      if (!request.current()) return { ok: false, conflict: false }
      state.notice = "Changes saved."
      return { ok: true, conflict: false }
    } catch {
      if (request.current())
        state.error = "Change could not be saved. Try again."
    } finally {
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
    if (state.groupConflict || !state.group) return
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
    )
    if (result.conflict && !disposed) state.groupConflict = true
  }
  async function addMember() {
    if (!state.principal) {
      state.error = "Choose a principal."
      return
    }
    const principal = state.principal
    await mutate(
      groupAdminPath(workspaceID(), groupID(), "/members"),
      "POST",
      { principal },
      async (_response, current) => {
        if (current()) {
          if (state.principal === principal) state.principal = ""
          await onMembersChanged()
        }
      },
    )
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
      async () => {
        await onMembersChanged()
      },
    )
    if (result.conflict && !disposed) await onMembersChanged()
  }
  async function createGrant() {
    await mutate(
      groupAdminPath(workspaceID(), groupID(), "/grants"),
      "POST",
      { role: state.newRole },
      async () => {
        await load(false)
      },
    )
  }
  function beginGrant(grant: SystemGroupGrant) {
    if (disposed || state.saving) return
    state.editingGrant = { ...grant }
    state.grantRole = grant.role
    state.grantEnabled = grant.enabled
    state.grantConflict = false
    state.error = state.notice = ""
  }
  function cancelGrant() {
    if (!state.saving) state.editingGrant = null
  }
  async function saveGrant() {
    if (!state.editingGrant || state.grantConflict) return
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
        await load(false)
      },
    )
    if (result.conflict && !disposed) {
      state.grantConflict = true
      state.error =
        "This grant changed elsewhere. Cancel and reopen it to use the latest revision."
      await load(false)
    }
  }
  async function toggleGrant(grant: SystemGroupGrant) {
    const result = await mutate(
      `workspaces/${encodeURIComponent(workspaceID())}/grants/${encodeURIComponent(grant.id)}`,
      "PATCH",
      { enabled: !grant.enabled, expected_revision: grant.revision },
      async (_response, current) => {
        if (current() && state.editingGrant?.id === grant.id)
          state.editingGrant = null
        await load(false)
      },
    )
    if (result.conflict && !disposed) await load(false)
  }
  function stop() {
    disposed = true
    resource.stop()
    mutation.stop()
  }
  return {
    state,
    load,
    create,
    saveGroup,
    addMember,
    toggleMember,
    createGrant,
    beginGrant,
    cancelGrant,
    saveGrant,
    toggleGrant,
    stop,
  }
}
