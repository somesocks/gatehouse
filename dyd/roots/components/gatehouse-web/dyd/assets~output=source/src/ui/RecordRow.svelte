<script lang="ts">
  import type { Snippet } from "svelte"
  let { children }: { children: Snippet } = $props()
</script>

<tr class="hover:bg-muted/30 focus-within:bg-muted/30 transition-colors"
  >{@render children()}</tr
>
