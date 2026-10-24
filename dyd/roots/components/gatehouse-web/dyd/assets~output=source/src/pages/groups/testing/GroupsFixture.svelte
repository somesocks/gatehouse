<script lang="ts">
  import { onMount, untrack } from "svelte"
  import { provideRuntime, type ApplicationRuntime } from "../../../app/runtime.svelte"
  import { parseRoute, type Route } from "../../../route"
  import GroupsRoute from "../GroupsRoute.svelte"

  let { initialPath, onNavigate, onReady, onSubscribe }: {
    initialPath: string
    onNavigate: (path: string) => void
    onReady?: (navigate: (path: string) => void) => void
    onSubscribe?: (refresh: () => Promise<void>) => void
  } = $props()
  const parse = (path: string) => parseRoute(new URL(path, "http://localhost"))
  const state = $state<{ route: Route }>({ route: untrack(() => parse(initialPath)) })
  provideRuntime({
    state,
    auth: { state: { status: "authenticated", claims: { principal: { ref: { id: "principal" } } } } },
    access: {
      state: { workspaceStatus: "ready", workspaces: [{ id: "workspace", name: "Workspace" }] },
      setSystemAccess() {},
    },
    activity: {
      subscribe(_topics: unknown, callback: (event: { signal: AbortSignal }) => Promise<void>) {
        onSubscribe?.(() => callback({ signal: new AbortController().signal }))
        return () => {}
      },
      poll: async () => {},
    },
    navigate(path: string) { onNavigate(path); state.route = parse(path) },
    requireLogin() {},
    refresh: async () => true,
  } as unknown as ApplicationRuntime)
  onMount(() => onReady?.((path) => { state.route = parse(path) }))
</script>

<GroupsRoute />
