<script lang="ts">
  import { onMount, untrack } from "svelte"
  import { provideRuntime, type ApplicationRuntime } from "../../app/runtime.svelte"
  import { parseRoute, type Route } from "../../route"
  import SessionWorkspace from "../../pages/session-chat/SessionWorkspace.svelte"

  let { onNavigate, initialRoute, onReady, onSubscribe }: {
    onNavigate: (path: string) => void
    initialRoute: Route
    onReady?: (navigate: (route: Route) => void) => void
    onSubscribe?: (refresh: () => Promise<void>) => void
  } = $props()
  const state = $state<{ route: Route }>({
    route: untrack(() => initialRoute),
  })
  provideRuntime({
    state,
    auth: { state: { status: "authenticated", claims: { principal: { ref: { id: "principal" } } } } },
    access: { state: { workspaceStatus: "ready", workspaces: [{ id: "workspace", name: "Workspace" }] } },
    activity: {
      subscribe(_topics: unknown, callback: (event: { signal: AbortSignal }) => Promise<void>) {
        onSubscribe?.(() => callback({ signal: new AbortController().signal }))
        return () => {}
      },
      poll: async () => {},
    },
    navigate(path: string) {
      onNavigate(path)
      state.route = parseRoute(new URL(path, "http://localhost"))
    },
    requireLogin() {},
    refresh: async () => true,
  } as unknown as ApplicationRuntime)
  onMount(() => onReady?.((route) => { state.route = route }))
</script>

<SessionWorkspace />
