import { sessionTool, type SessionWorkspaceRoute } from "./session-tools"

// Route navigation closes the dialog immediately, but its last resource remains
// mounted until the dialog primitive reports that the exit has completed.
export function createResourcePresentation() {
  const state = $state({
    route: null as SessionWorkspaceRoute | null,
    open: false,
  })
  let owner = ""

  return {
    state,
    update(route: SessionWorkspaceRoute, allowed: boolean, principalID?: string) {
      const nextOwner = JSON.stringify([principalID, route.workspaceID, route.sessionID])
      if (!allowed || owner !== nextOwner) {
        state.route = null
        state.open = false
        owner = allowed ? nextOwner : ""
      }
      if (!allowed) return
      if (sessionTool(route) !== null) {
        state.route = route
        state.open = true
      } else {
        state.open = false
      }
    },
    closed() {
      // An obsolete completion callback must not remove a newly opened view.
      if (!state.open) state.route = null
    },
  }
}
