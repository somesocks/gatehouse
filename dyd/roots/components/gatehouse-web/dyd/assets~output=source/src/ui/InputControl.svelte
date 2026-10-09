<script lang="ts">
  import type { Snippet } from "svelte"
  import Spinner from "./Spinner.svelte"
  let {
    loading = false,
    loadingLabel = "Saving",
    trailing,
    children,
  }: {
    loading?: boolean
    loadingLabel?: string
    trailing?: Snippet
    children: Snippet
  } = $props()
</script>

<div class="flex min-w-0 items-center gap-2">
  {@render children()}{#if loading}<Spinner
      role="status"
      aria-label={loadingLabel}
    />{/if}{#if trailing}{@render trailing()}{/if}
</div>
