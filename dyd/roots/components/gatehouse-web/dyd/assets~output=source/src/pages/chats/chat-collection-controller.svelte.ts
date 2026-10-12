import { fetchChats, type Chat, type ChatSearchResponse } from "../../app/chats"
import { createDirectoryController } from "../../utils/directory-controller.svelte"

export function createChatCollectionController({
  onAuthenticationLost,
}: {
  onAuthenticationLost: () => void
}) {
  let owner = ""
  let parent: AbortSignal | undefined
  const draft = $state({ projectID: "" })
  const directory = createDirectoryController<Chat>({
    onAuthenticationLost,
    async fetchPage(name, cursor, projectID, signal) {
      const response = await fetchChats(
        owner,
        name,
        cursor,
        parent ? AbortSignal.any([parent, signal]) : signal,
        projectID,
      )
      if (!response.ok) return { response }
      const result = (await response.json()) as ChatSearchResponse
      return { response, items: result.sessions, next: result.next_cursor }
    },
  })
  const state = $state({
    get chats() {
      return directory.state.items
    },
    get search() {
      return directory.state.search
    },
    set search(value: string) {
      directory.state.search = value
    },
    get projectID() {
      return draft.projectID
    },
    set projectID(value: string) {
      draft.projectID = value
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
  function start(
    workspaceID: string,
    search: string,
    signal: AbortSignal,
    projectID = "",
  ) {
    directory.stop()
    if (owner !== workspaceID) directory.reset()
    owner = workspaceID
    parent = signal
    state.search = search
    state.projectID = projectID
    directory.state.status = projectID
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
