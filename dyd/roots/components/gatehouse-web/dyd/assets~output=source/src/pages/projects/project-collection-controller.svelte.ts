import {
  fetchProjects,
  type ProjectSearchResponse,
  type ProjectSummary,
} from "../../app/projects"
import { createDirectoryController } from "../../utils/directory-controller.svelte"

export function createProjectCollectionController({
  onAuthenticationLost,
}: {
  onAuthenticationLost: () => void
}) {
  let owner = ""
  let parent: AbortSignal | undefined
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
    directory.stop()
    if (owner !== workspaceID) directory.reset()
    owner = workspaceID
    parent = signal
    state.search = search
    void directory.search()
    return directory.stop
  }
  return {
    state,
    start,
    stop: directory.stop,
    reload: directory.reload,
    retry: directory.retry,
    search: directory.search,
    loadMore: (_workspaceID: string, _search: string, _signal: AbortSignal) =>
      directory.next(),
    previous: (_workspaceID: string, _search: string, _signal: AbortSignal) =>
      directory.previous(),
  }
}
