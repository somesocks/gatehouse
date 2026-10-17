<script lang="ts">
  import type { Snippet } from "svelte"
  let { children }: { children: Snippet } = $props()
</script>

<div
  role="group"
  aria-label="Selected files"
  class="flex max-h-32 min-w-0 flex-wrap gap-2 overflow-y-auto overscroll-contain p-1"
>
  {@render children()}
</div>
