<script lang="ts">
  import type { Snippet } from "svelte"
  import ResourceActions from "./ResourceActions.svelte"
  let { children, dialog = false }: { children: Snippet; dialog?: boolean } = $props()
</script>

{#if dialog}
  <ResourceActions includeClose={false}>{@render children()}</ResourceActions>
{:else}
<footer
  class="sticky bottom-0 z-10 flex flex-wrap items-center justify-end gap-3 rounded-b-lg border-t bg-card px-6 py-4 [&>a]:order-first"
>
  {@render children()}
</footer>
{/if}
