<script lang="ts">
  import type { Snippet } from "svelte"
  let { children }: { children: Snippet } = $props()
</script>

<footer
  class="sticky bottom-0 z-20 shrink-0 border-t bg-card p-4 md:rounded-b-xl md:px-6"
>
  {@render children()}
</footer>
