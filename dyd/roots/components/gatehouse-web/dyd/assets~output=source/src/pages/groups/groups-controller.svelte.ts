import type { ActivityClient } from "../../app/activity"
import { fetchWorkspaceGroups, type Group } from "../../app/groups"
import { createCollectionPage } from "../../utils/collection-page.svelte"
import { createLoadState } from "../../utils/load-state.svelte"

type GroupsControllerOptions = {
  activity: ActivityClient
  onAuthenticationLost: () => void
}

export function createGroupsController({
  activity,
  onAuthenticationLost,
}: GroupsControllerOptions) {
  const resource = createLoadState()
  const state = $state({
    groups: [] as Group[],
    search: "",
    get loading() {
      return resource.loading
    },
    get reloading() {
      return resource.reloading
    },
    get busy() {
      return resource.state.pending
    },
    get hasLoaded() {
      return resource.state.hasLoaded
    },
    get error() {
      return resource.state.error
    },
  })
  const page = createCollectionPage()
  let unsubscribe: (() => void) | undefined
  let generation = 0

  async function load(
    workspaceID: string,
    signal: AbortSignal,
    currentGeneration = generation,
  ): Promise<boolean> {
    const request = resource.begin(signal)
    signal = request.signal
    try {
      const requested = page.key()
      const response = await fetchWorkspaceGroups(
        workspaceID,
        signal,
        page.params(),
      )
      if (currentGeneration !== generation || !request.current()) {
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
      if (currentGeneration !== generation || !request.current()) {
        return false
      }
      if (requested !== page.key()) return false
      return request.commit(() => {
        state.groups = groups
        page.accept(response)
      })
    } catch (cause) {
      if (currentGeneration === generation && !signal.aborted)
        request.fail(cause)
      return false
    } finally {
      request.finish()
    }
  }

  function start(workspaceID: string, routeSignal: AbortSignal): () => void {
    stop()
    page.reset()
    resource.reset()
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
    resource.stop()
    unsubscribe?.()
    unsubscribe = undefined
  }

  return { state, page, load, start, stop }
}
