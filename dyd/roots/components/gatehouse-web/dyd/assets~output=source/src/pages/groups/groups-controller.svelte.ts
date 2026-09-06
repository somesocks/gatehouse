import type { ActivityClient } from "../../app/activity"
import { fetchWorkspaceGroups, type Group } from "../../app/groups"

type GroupsControllerOptions = {
  activity: ActivityClient
  onAuthenticationLost: () => void
}

export function createGroupsController({ activity, onAuthenticationLost }: GroupsControllerOptions) {
  const state = $state({ groups: [] as Group[], search: "" })
  let unsubscribe: (() => void) | undefined
  let generation = 0

  async function load(workspaceID: string, signal: AbortSignal, currentGeneration = generation): Promise<boolean> {
    const response = await fetchWorkspaceGroups(workspaceID, signal)
    if (currentGeneration !== generation || signal.aborted) {
      return false
    }
    if (response.status === 401) {
      onAuthenticationLost()
      return false
    }
    if (!response.ok) {
      throw new Error("groups could not be refreshed")
    }
    const groups = await response.json() as Group[]
    if (currentGeneration !== generation || signal.aborted) {
      return false
    }
    state.groups = groups
    return true
  }

  function start(workspaceID: string, routeSignal: AbortSignal): () => void {
    stop()
    const currentGeneration = ++generation
    state.groups = []
    void load(workspaceID, routeSignal, currentGeneration)
    unsubscribe = activity.subscribe([{ name: "group", topic: workspaceID, events: ["group.*", "group_member.*"] }], async ({ signal }) => {
      if (signal.aborted || routeSignal.aborted) {
        return
      }
      if (!await load(workspaceID, routeSignal, currentGeneration) || signal.aborted || routeSignal.aborted) {
        throw new Error("groups refresh failed")
      }
    })
    return stop
  }

  function stop(): void {
    generation += 1
    unsubscribe?.()
    unsubscribe = undefined
  }

  return { state, start, stop }
}
