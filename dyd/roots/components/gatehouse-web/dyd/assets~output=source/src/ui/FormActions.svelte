<script lang="ts">
  import type { Snippet } from "svelte"
  let { children }: { children: Snippet } = $props()
</script>

<footer
  class="sticky bottom-0 z-10 flex flex-wrap items-center justify-end gap-3 rounded-b-lg border-t bg-card px-6 py-4 [&>a]:order-first"
>
  {@render children()}
</footer>
