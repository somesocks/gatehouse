import {
  fetchProjects,
  type ProjectSearchResponse,
  type ProjectSummary,
} from "../../app/projects"
import { createDirectoryController } from "../../utils/directory-controller.svelte"
import type { ActivityClient } from "../../app/activity"

export function createProjectCollectionController({
  onAuthenticationLost,
  activity,
}: {
  onAuthenticationLost: () => void
  activity?: ActivityClient
}) {
  let owner = ""
  let parent: AbortSignal | undefined
  let unsubscribe: (() => void) | undefined
  const directory = createDirectoryController<ProjectSummary>({
    onAuthenticationLost,
    async fetchPage(name, cursor, _status, signal) {
      const response = await fetchProjects(
        owner,
        name,
        cursor,
        parent ? AbortSignal.any([parent, signal]) : signal,
      )
      if (!response.ok) return { response }
      const result = (await response.json()) as ProjectSearchResponse
      return { response, items: result.projects, next: result.next_cursor }
    },
  })
  const state = $state({
    get projects() {
      return directory.state.items
    },
    get search() {
      return directory.state.search
    },
    set search(value: string) {
      directory.state.search = value
    },
    get cursor() {
      return directory.state.next || null
    },
    get page() {
      return directory.state.cursor
    },
    get history() {
      return directory.state.history
    },
    get loading() {
      return directory.state.loading
    },
    get reloading() {
      return directory.state.reloading
    },
    get busy() {
      return directory.state.busy
    },
    get hasLoaded() {
      return directory.state.hasLoaded
    },
    get error() {
      return directory.state.error
    },
  })
  function start(workspaceID: string, search: string, signal: AbortSignal) {
    stop()
    if (owner !== workspaceID) directory.reset()
    owner = workspaceID
    parent = signal
    state.search = search
    void directory.search()
    unsubscribe = activity?.subscribe([
      { name: "project-directory-access", topic: workspaceID, events: ["project.*", "project_grant.*", "group.*", "group_member.*", "workspace_grant.*"] },
    ], async ({ signal: eventSignal }) => {
      if (parent !== signal || signal.aborted || eventSignal.aborted) return
      if (!(await directory.reload()) && !signal.aborted && !eventSignal.aborted)
        throw new Error("Projects could not be refreshed.")
    })
    void activity?.poll()
    return stop
  }
  function stop() {
    directory.stop()
    unsubscribe?.()
    unsubscribe = undefined
  }
  return {
    state,
    start,
    stop,
    reload: directory.reload,
    retry: directory.retry,
    search: directory.search,
    loadMore: (_workspaceID: string, _search: string, _signal: AbortSignal) =>
      directory.next(),
    previous: (_workspaceID: string, _search: string, _signal: AbortSignal) =>
      directory.previous(),
  }
}
