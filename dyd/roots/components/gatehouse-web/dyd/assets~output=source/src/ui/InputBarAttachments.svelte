<script lang="ts">
  import type { Snippet } from "svelte"
  let { children, label = "Selected files" }: { children: Snippet; label?: string } = $props()
</script>

<div
  role="group"
  aria-label={label}
  class="flex max-h-32 min-w-0 items-center gap-2 overflow-x-auto overflow-y-hidden overscroll-contain p-1 [&>*]:shrink-0 md:flex-wrap md:overflow-y-auto"
>
  {@render children()}
</div>
