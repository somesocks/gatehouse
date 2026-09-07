import type { ActivityClient } from "../../app/activity"
import { createSystemGrant, fetchSystemGrants, fetchSystemPrincipals, systemAdministration, updateSystemGrant, updateSystemPrincipal, type SystemAgentModel, type SystemGrant, type SystemPrincipal, type SystemStorageProvider, type SystemWorkspaceAgent, type SystemWorkspaceStorageProvider } from "../../app/system"

type SystemRouteKind = "system" | "system-grants" | "system-principals" | "system-agent-models" | "system-storage-providers" | "system-workspace-bindings"
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
    agentModels: [] as SystemAgentModel[],
    storageProviders: [] as SystemStorageProvider[],
    workspaceAgents: [] as SystemWorkspaceAgent[],
    workspaceStorageProviders: [] as SystemWorkspaceStorageProvider[],
    administrationError: "",
    savingAdministration: false,
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
    if (!await loadGrants() || route === "system" || route === "system-grants") {
      return
    }
    if (route === "system-principals") { await loadPrincipals(); return }
    if (route === "system-agent-models") { await loadAdministration("agent-models", "agentModels"); return }
    if (route === "system-storage-providers") { await loadAdministration("storage-providers", "storageProviders"); return }
    if (route === "system-workspace-bindings") {
      await Promise.all([loadAdministration("workspace-agents", "workspaceAgents"), loadAdministration("workspace-storage-providers", "workspaceStorageProviders")])
    }
  }

  async function loadAdministration(path: string, field: "agentModels" | "storageProviders" | "workspaceAgents" | "workspaceStorageProviders"): Promise<void> {
    state.administrationError = ""
    try {
      const response = await systemAdministration(path)
      if (response.status === 401) { onAuthenticationLost(); return }
      if (response.status === 403) { onSystemAccessChange("denied"); return }
      if (!response.ok) { state.administrationError = "System settings could not be loaded."; return }
      ;(state[field] as unknown) = await response.json()
    } catch { state.administrationError = "System settings could not be loaded." }
  }

  async function saveAdministration(path: string, method: "POST" | "PATCH", body: Record<string, unknown>, reload: () => Promise<void>): Promise<boolean> {
    state.administrationError = ""
    state.savingAdministration = true
    try {
      const response = await systemAdministration(path, method, body)
      if (response.status === 401) { onAuthenticationLost(); return false }
      if (response.status === 403) { onSystemAccessChange("denied"); return false }
      if (response.status === 409) { state.administrationError = "This setting changed elsewhere. The latest settings have been reloaded."; await reload(); return false }
      if (!response.ok) { state.administrationError = "System setting could not be saved."; return false }
      await reload()
      return true
    } catch { state.administrationError = "System setting could not be saved."; return false } finally { state.savingAdministration = false }
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

  function start(route: string): () => void {
    stop()
    const refresh = route === "system-grants"
      ? [{ name: "system-grants", topic: "sys", events: ["system_grant.*"] }]
      : route === "system-agent-models"
          ? [{ name: "agent-models", topic: "sys", events: ["agent_model.*"] }]
          : route === "system-storage-providers"
            ? [{ name: "storage-providers", topic: "sys", events: ["storage_provider.*"] }]
            : route === "system-workspace-bindings"
              ? [{ name: "workspace-agents", topic: "sys", events: ["workspace_agent.*"] }, { name: "workspace-storage-providers", topic: "sys", events: ["workspace_storage_provider.*"] }]
              : []
    if (refresh.length === 0) {
      return stop
    }
    unsubscribe = activity.subscribe(refresh, async ({ names }) => {
      if (names.has("system-grants") && !await loadGrants()) throw new Error("system grants refresh failed")
      if (names.has("agent-models")) await loadAdministration("agent-models", "agentModels")
      if (names.has("storage-providers")) await loadAdministration("storage-providers", "storageProviders")
      if (names.has("workspace-agents")) await loadAdministration("workspace-agents", "workspaceAgents")
      if (names.has("workspace-storage-providers")) await loadAdministration("workspace-storage-providers", "workspaceStorageProviders")
    })
    return stop
  }

  function stop(): void {
    unsubscribe?.()
    unsubscribe = undefined
  }

  return { state, load, loadAdministration, saveAdministration, setPrincipalEnabled, addGrant, setGrantEnabled, start, stop }
}
