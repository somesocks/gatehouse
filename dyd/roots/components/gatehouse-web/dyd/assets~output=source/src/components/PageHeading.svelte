<script lang="ts">
  import type { Snippet } from "svelte"

  let {
    as = "div",
    actions,
    children,
  }: {
    as?: "div" | "header"
    actions?: Snippet
    children: Snippet
  } = $props()
</script>

<svelte:element this={as} class="page-heading split">
  <div class="stack">
    {@render children()}
  </div>
  {#if actions}
    <div class="page-heading-actions cluster">
      {@render actions()}
    </div>
  {/if}
</svelte:element>
