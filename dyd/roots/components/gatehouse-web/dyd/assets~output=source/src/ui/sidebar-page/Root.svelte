<script lang="ts">
  import { Provider } from "../../lib/vendor/shadcn/sidebar"
  import type { Snippet } from "svelte"
  let {
    viewport = false,
    children,
  }: { viewport?: boolean; children: Snippet } = $props()
</script>

<Provider class={viewport ? "h-svh min-h-0 min-w-0 overflow-x-clip" : "min-w-0"}
  >{@render children()}</Provider
>
