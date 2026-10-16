import { systemAdministration, type SystemPrincipal } from "../../../app/system"
import { createLoadState } from "../../../utils/load-state.svelte"

export function createPrincipalNewController({
  onAuthenticationLost,
  onSystemAccessChange,
}: {
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const mutation = createLoadState()
  let disposed = false
  const state = $state({
    name: "",
    alias: "",
    enabled: true,
    error: "",
    get saving() {
      return mutation.state.pending
    },
  })
  async function create(): Promise<SystemPrincipal | null> {
    if (disposed || state.saving) return null
    const request = mutation.begin()
    state.error = ""
    try {
      const response = await systemAdministration(
        "principals",
        "POST",
        {
          name: state.name.trim() || undefined,
          alias: state.alias.trim() || undefined,
          enabled: state.enabled,
        },
        request.signal,
      )
      if (!request.current()) return null
      if (response.status === 401) onAuthenticationLost()
      else if (response.status === 403) onSystemAccessChange("denied")
      else if (response.status === 409)
        state.error = "That principal alias is already in use."
      else if (response.status === 400) {
        const message = await response.text()
        if (request.current())
          state.error = message.trim() || "Check the principal details."
      } else if (!response.ok) state.error = "Principal could not be created."
      else {
        const principal = (await response.json()) as SystemPrincipal
        return request.current() ? principal : null
      }
    } catch {
      if (request.current()) state.error = "Principal could not be created."
    } finally {
      request.finish()
    }
    return null
  }
  function stop() {
    disposed = true
    mutation.stop()
  }
  return { state, create, stop }
}
