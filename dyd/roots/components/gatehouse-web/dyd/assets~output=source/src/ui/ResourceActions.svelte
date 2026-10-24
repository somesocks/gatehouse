<script lang="ts">
  import type { Snippet } from "svelte"
  let { children }: {
    children: Snippet
  } = $props()
</script>

<div class="flex flex-wrap items-center justify-end gap-2">{@render children()}</div>
