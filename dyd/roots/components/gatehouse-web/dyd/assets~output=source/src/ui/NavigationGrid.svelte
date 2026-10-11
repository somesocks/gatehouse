<script lang="ts">
  import type { Snippet } from "svelte"
  let { children }: { children: Snippet } = $props()
</script>

<div class="grid gap-4 sm:grid-cols-2">{@render children()}</div>
