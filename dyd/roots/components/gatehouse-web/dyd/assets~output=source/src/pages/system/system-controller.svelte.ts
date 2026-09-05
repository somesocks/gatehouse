import type { ActivityClient } from "../../app/activity"
import { createSystemGrant, fetchSystemGrants, fetchSystemPrincipals, updateSystemGrant, updateSystemPrincipal, type SystemGrant, type SystemPrincipal } from "../../app/system"

type SystemRouteKind = "system" | "system-grants" | "system-principals"
type SystemAccess = "available" | "denied" | "unavailable"

type SystemControllerOptions = {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: SystemAccess) => void
  principalID: () => string | undefined
}

export function createSystemController({ activity, onAuthenticationLost, onSystemAccessChange, principalID }: SystemControllerOptions) {
  const state = $state({
    grants: [] as SystemGrant[],
    principals: [] as SystemPrincipal[],
    grantPrincipal: "",
    grantError: "",
    principalError: "",
    creatingGrant: false,
    updatingGrantIDs: new Set<string>(),
    updatingPrincipalIDs: new Set<string>(),
  })
  let unsubscribe: (() => void) | undefined

  async function loadGrants(): Promise<boolean> {
    state.grantError = ""
    try {
      const response = await fetchSystemGrants()
      if (response.status === 401) {
        onAuthenticationLost()
        return false
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        state.grants = []
        return false
      }
      if (!response.ok) {
        onSystemAccessChange("unavailable")
        state.grantError = "System grants could not be loaded."
        return false
      }
      state.grants = await response.json() as SystemGrant[]
      onSystemAccessChange("available")
      return true
    } catch {
      onSystemAccessChange("unavailable")
      state.grantError = "System grants could not be loaded."
      return false
    }
  }

  async function loadPrincipals(): Promise<boolean> {
    state.principalError = ""
    try {
      const response = await fetchSystemPrincipals()
      if (response.status === 401) {
        onAuthenticationLost()
        return false
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        state.principals = []
        return false
      }
      if (!response.ok) {
        state.principalError = "Principals could not be loaded."
        return false
      }
      state.principals = await response.json() as SystemPrincipal[]
      return true
    } catch {
      state.principalError = "Principals could not be loaded."
      return false
    }
  }

  async function load(route: SystemRouteKind): Promise<void> {
    if (!await loadGrants() || route !== "system-principals") {
      return
    }
    await loadPrincipals()
  }

  async function setPrincipalEnabled(principal: SystemPrincipal, enabled: boolean): Promise<void> {
    if (!enabled && !window.confirm(`Disable ${principal.name ?? principal.id}?`)) {
      return
    }
    state.principalError = ""
    state.updatingPrincipalIDs = new Set(state.updatingPrincipalIDs).add(principal.id)
    try {
      const response = await updateSystemPrincipal(principal.id, enabled)
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        state.principals = []
        return
      }
      if (!response.ok) {
        state.principalError = "Principal could not be updated."
        return
      }
      const updated = await response.json() as SystemPrincipal
      state.principals = state.principals.map((entry) => entry.id === updated.id ? updated : entry)
      if (!updated.enabled && principalID() === updated.id) {
        onAuthenticationLost()
      }
    } catch {
      state.principalError = "Principal could not be updated."
    } finally {
      const next = new Set(state.updatingPrincipalIDs)
      next.delete(principal.id)
      state.updatingPrincipalIDs = next
    }
  }

  async function addGrant(): Promise<void> {
    state.grantError = ""
    const principal = state.grantPrincipal.trim()
    if (principal === "") {
      state.grantError = "A principal ID is required."
      return
    }
    state.creatingGrant = true
    try {
      const response = await createSystemGrant(principal)
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        state.grants = []
        return
      }
      if (response.status === 404) {
        state.grantError = "That principal does not exist."
        return
      }
      if (response.status === 409) {
        state.grantError = "That principal already has a system grant."
        return
      }
      if (!response.ok) {
        state.grantError = "System grant could not be created."
        return
      }
      const grant = await response.json() as SystemGrant
      state.grants = [...state.grants, grant].sort((left, right) => left.ref.id.localeCompare(right.ref.id))
      state.grantPrincipal = ""
    } catch {
      state.grantError = "System grant could not be created."
    } finally {
      state.creatingGrant = false
    }
  }

  async function setGrantEnabled(grant: SystemGrant, enabled: boolean): Promise<void> {
    if (!enabled && !window.confirm(`Disable system access for ${grant.principal.id}?`)) {
      return
    }
    state.grantError = ""
    state.updatingGrantIDs = new Set(state.updatingGrantIDs).add(grant.ref.id)
    try {
      const response = await updateSystemGrant(grant.ref.id, enabled)
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        state.grants = []
        return
      }
      if (!response.ok) {
        state.grantError = "System grant could not be updated."
        return
      }
      const updated = await response.json() as SystemGrant
      state.grants = state.grants.map((entry) => entry.ref.id === updated.ref.id ? updated : entry)
      if (!updated.enabled && principalID() === updated.principal.id) {
        onSystemAccessChange("denied")
        state.grants = []
      }
    } catch {
      state.grantError = "System grant could not be updated."
    } finally {
      const next = new Set(state.updatingGrantIDs)
      next.delete(grant.ref.id)
      state.updatingGrantIDs = next
    }
  }

  function start(): () => void {
    stop()
    unsubscribe = activity.subscribe([{ name: "system", topic: "sys", events: ["system_grant.*"] }], async () => {
      if (!await loadGrants()) {
        throw new Error("system grants refresh failed")
      }
    })
    return stop
  }

  function stop(): void {
    unsubscribe?.()
    unsubscribe = undefined
  }

  return { state, load, setPrincipalEnabled, addGrant, setGrantEnabled, start, stop }
}
