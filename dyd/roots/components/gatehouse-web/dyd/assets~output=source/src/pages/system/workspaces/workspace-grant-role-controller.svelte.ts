import {
  updateSystemWorkspaceGrantRole,
  type SystemWorkspacePrincipalGrant,
  type WorkspaceRole,
} from "../../../app/system"
import { createLoadState } from "../../../utils/load-state.svelte"

export function createWorkspaceGrantRoleController({
  workspaceID,
  onAuthenticationLost,
  onSystemAccessChange,
  onReload,
}: {
  workspaceID: () => string
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
  onReload: () => Promise<unknown>
}) {
  const mutation = createLoadState()
  let disposed = false
  const state = $state({
    grant: null as SystemWorkspacePrincipalGrant | null,
    recipient: "",
    role: "guest" as WorkspaceRole,
    error: "",
    notice: "",
    conflict: false,
    get saving() {
      return mutation.state.pending
    },
  })
  function cancel() {
    if (state.saving) return
    state.grant = null
    state.recipient = ""
    state.error = ""
    state.conflict = false
  }
  function begin(grant: SystemWorkspacePrincipalGrant, recipient: string) {
    if (disposed || state.saving || grant.workspace !== workspaceID()) return
    cancel()
    state.notice = ""
    state.grant = { ...grant }
    state.recipient = recipient
    state.role = grant.role
  }
  async function save() {
    if (disposed || state.saving || !state.grant || state.conflict) return
    const grant = state.grant
    const request = mutation.begin()
    state.error = state.notice = ""
    try {
      const response = await updateSystemWorkspaceGrantRole(
        workspaceID(),
        grant.id,
        state.role,
        grant.revision,
        request.signal,
      )
      if (!request.current()) return
      if (response.status === 401) onAuthenticationLost()
      else if (response.status === 403) onSystemAccessChange("denied")
      else if (response.status === 409) {
        const message = await response.text()
        if (!request.current()) return
        state.conflict = message.includes("changed elsewhere")
        state.error = state.conflict
          ? "This grant changed elsewhere. Cancel and reopen it to use the latest revision."
          : message.trim() ||
            "Another grant for this recipient already has that role."
        await onReload()
      } else if (response.status === 404) {
        state.conflict = true
        state.error = "This grant no longer exists. Cancel and reload the list."
        await onReload()
      } else if (!response.ok) state.error = "Grant role could not be saved."
      else {
        state.grant = null
        state.conflict = false
        state.notice =
          "Grant role updated. Group-derived permissions may also apply."
        await onReload()
      }
    } catch {
      if (request.current()) state.error = "Grant role could not be saved."
    } finally {
      request.finish()
    }
  }
  function stop() {
    disposed = true
    mutation.stop()
    state.grant = null
  }
  return { state, begin, cancel, save, stop }
}
