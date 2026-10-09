<script lang="ts">
  import { Provider } from "../../lib/vendor/shadcn/sidebar"
  import type { Snippet } from "svelte"
  let { children }: { children: Snippet } = $props()
</script>

<Provider>{@render children()}</Provider>
