import type { ActivityClient } from "../../app/activity"
import { fetchWorkspaceGroups, type Group } from "../../app/groups"
import { createCollectionPage } from "../../utils/collection-page.svelte"

type GroupsControllerOptions = {
  activity: ActivityClient
  onAuthenticationLost: () => void
}

export function createGroupsController({
  activity,
  onAuthenticationLost,
}: GroupsControllerOptions) {
  const state = $state({
    groups: [] as Group[],
    search: "",
    loading: false,
    error: "",
  })
  const page = createCollectionPage()
  let unsubscribe: (() => void) | undefined
  let generation = 0

  async function load(
    workspaceID: string,
    signal: AbortSignal,
    currentGeneration = generation,
  ): Promise<boolean> {
    state.loading = true
    state.error = ""
    try {
      const requested = page.key()
      const response = await fetchWorkspaceGroups(
        workspaceID,
        signal,
        page.params(),
      )
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
      const groups = (await response.json()) as Group[]
      if (currentGeneration !== generation || signal.aborted) {
        return false
      }
      if (requested !== page.key()) return false
      state.groups = groups
      page.accept(response)
      return true
    } catch (cause) {
      if (currentGeneration === generation && !signal.aborted)
        state.error =
          cause instanceof Error ? cause.message : "Groups could not be loaded."
      return false
    } finally {
      if (currentGeneration === generation) state.loading = false
    }
  }

  function start(workspaceID: string, routeSignal: AbortSignal): () => void {
    stop()
    page.reset()
    const currentGeneration = ++generation
    state.groups = []
    void load(workspaceID, routeSignal, currentGeneration)
    unsubscribe = activity.subscribe(
      [
        {
          name: "group",
          topic: workspaceID,
          events: ["group.*", "group_member.*"],
        },
      ],
      async ({ signal }) => {
        if (signal.aborted || routeSignal.aborted) {
          return
        }
        if (
          !(await load(workspaceID, routeSignal, currentGeneration)) ||
          signal.aborted ||
          routeSignal.aborted
        ) {
          throw new Error("groups refresh failed")
        }
      },
    )
    return stop
  }

  function stop(): void {
    generation += 1
    unsubscribe?.()
    unsubscribe = undefined
  }

  return { state, page, load, start, stop }
}
