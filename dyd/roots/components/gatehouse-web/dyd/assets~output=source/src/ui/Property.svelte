<script lang="ts">
  import type { Snippet } from "svelte"
  let { label, children }: { label: string; children: Snippet } = $props()
</script>

<div
  class="grid gap-2 py-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,3fr)] sm:gap-8"
>
  <dt class="text-muted-foreground text-sm">{label}</dt>
  <dd class="min-w-0 text-sm leading-relaxed break-words">
    {@render children()}
  </dd>
</div>
