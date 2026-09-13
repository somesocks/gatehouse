import {
  fetchSystemPrincipals,
  updateSystemPrincipal,
  type SystemPrincipal,
} from "../../../app/system"

type PrincipalsControllerOptions = {
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
  principalID: () => string | undefined
}

export function createPrincipalsController({
  onAuthenticationLost,
  onSystemAccessChange,
  principalID,
}: PrincipalsControllerOptions) {
  const state = $state({
    principals: [] as SystemPrincipal[],
    error: "",
    updatingIDs: new Set<string>(),
  })

  async function load(): Promise<void> {
    state.error = ""
    try {
      const response = await fetchSystemPrincipals()
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
        state.error = "Principals could not be loaded."
        return
      }
      state.principals = (await response.json()) as SystemPrincipal[]
    } catch {
      state.error = "Principals could not be loaded."
    }
  }

  async function setEnabled(
    principal: SystemPrincipal,
    enabled: boolean,
  ): Promise<void> {
    if (
      !enabled &&
      !window.confirm(`Disable ${principal.name ?? principal.id}?`)
    )
      return
    state.error = ""
    state.updatingIDs = new Set(state.updatingIDs).add(principal.id)
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
        state.error = "Principal could not be updated."
        return
      }
      const updated = (await response.json()) as SystemPrincipal
      state.principals = state.principals.map((entry) =>
        entry.id === updated.id ? updated : entry,
      )
      if (!updated.enabled && principalID() === updated.id)
        onAuthenticationLost()
    } catch {
      state.error = "Principal could not be updated."
    } finally {
      const next = new Set(state.updatingIDs)
      next.delete(principal.id)
      state.updatingIDs = next
    }
  }

  return { state, load, setEnabled }
}
