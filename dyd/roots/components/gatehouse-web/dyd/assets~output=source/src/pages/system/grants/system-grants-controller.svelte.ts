import type { ActivityClient } from "../../../app/activity"
import { createSystemGrant, fetchSystemGrants, updateSystemGrant, type SystemGrant } from "../../../app/system"

type SystemGrantsControllerOptions = {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
  principalID: () => string | undefined
}

export function createSystemGrantsController({ activity, onAuthenticationLost, onSystemAccessChange, principalID }: SystemGrantsControllerOptions) {
  const state = $state({ grants: [] as SystemGrant[], principal: "", error: "", creating: false, updatingIDs: new Set<string>() })
  let unsubscribe: (() => void) | undefined

  async function load(): Promise<boolean> {
    state.error = ""
    try {
      const response = await fetchSystemGrants()
      if (response.status === 401) { onAuthenticationLost(); return false }
      if (response.status === 403) { onSystemAccessChange("denied"); state.grants = []; return false }
      if (!response.ok) { onSystemAccessChange("unavailable"); state.error = "System grants could not be loaded."; return false }
      state.grants = await response.json() as SystemGrant[]
      onSystemAccessChange("available")
      return true
    } catch { onSystemAccessChange("unavailable"); state.error = "System grants could not be loaded."; return false }
  }

  async function create(): Promise<void> {
    state.error = ""
    const principal = state.principal.trim()
    if (principal === "") { state.error = "A principal ID is required."; return }
    state.creating = true
    try {
      const response = await createSystemGrant(principal)
      if (response.status === 401) { onAuthenticationLost(); return }
      if (response.status === 403) { onSystemAccessChange("denied"); state.grants = []; return }
      if (response.status === 404) { state.error = "That principal does not exist."; return }
      if (response.status === 409) { state.error = "That principal already has a system grant."; return }
      if (!response.ok) { state.error = "System grant could not be created."; return }
      const grant = await response.json() as SystemGrant
      state.grants = [...state.grants, grant].sort((left, right) => left.ref.id.localeCompare(right.ref.id))
      state.principal = ""
    } catch { state.error = "System grant could not be created." } finally { state.creating = false }
  }

  async function setEnabled(grant: SystemGrant, enabled: boolean): Promise<void> {
    if (!enabled && !window.confirm(`Disable system access for ${grant.principal.id}?`)) return
    state.error = ""
    state.updatingIDs = new Set(state.updatingIDs).add(grant.ref.id)
    try {
      const response = await updateSystemGrant(grant.ref.id, enabled)
      if (response.status === 401) { onAuthenticationLost(); return }
      if (response.status === 403) { onSystemAccessChange("denied"); state.grants = []; return }
      if (!response.ok) { state.error = "System grant could not be updated."; return }
      const updated = await response.json() as SystemGrant
      state.grants = state.grants.map((entry) => entry.ref.id === updated.ref.id ? updated : entry)
      if (!updated.enabled && principalID() === updated.principal.id) { onSystemAccessChange("denied"); state.grants = [] }
    } catch { state.error = "System grant could not be updated." } finally {
      const next = new Set(state.updatingIDs)
      next.delete(grant.ref.id)
      state.updatingIDs = next
    }
  }

  function start(): () => void {
    stop()
    unsubscribe = activity.subscribe([{ name: "system-grants", topic: "sys", events: ["system_grant.*"] }], async () => { if (!await load()) throw new Error("system grants refresh failed") })
    return stop
  }

  function stop(): void { unsubscribe?.(); unsubscribe = undefined }

  return { state, load, create, setEnabled, start, stop }
}
