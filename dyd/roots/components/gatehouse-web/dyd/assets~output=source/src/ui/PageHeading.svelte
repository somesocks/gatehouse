<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    as = "header",
    actions,
    children,
  }: { as?: "div" | "header"; actions?: Snippet; children: Snippet } = $props()
</script>

<svelte:element
  this={as}
  class="flex flex-col gap-4 border-b pb-5 sm:flex-row sm:items-start sm:justify-between"
>
  <div class="min-w-0 flex-1 space-y-2">{@render children()}</div>
  {#if actions}<div class="flex shrink-0 flex-wrap items-center gap-2">
      {@render actions()}
    </div>{/if}
</svelte:element>
