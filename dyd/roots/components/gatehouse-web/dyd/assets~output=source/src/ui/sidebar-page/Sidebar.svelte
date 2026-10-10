<script lang="ts">
  import { Root } from "../../lib/vendor/shadcn/sidebar"
  import type { Snippet } from "svelte"
  let { children }: { children: Snippet } = $props()
</script>

<Root variant="inset" collapsible="offcanvas">{@render children()}</Root>
