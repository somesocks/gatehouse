<script lang="ts">
  import type { Snippet } from "svelte"

  let {
    children,
    trailing,
    loading = false,
    loadingLabel = "Loading",
  }: {
    children: Snippet
    trailing?: Snippet
    loading?: boolean
    loadingLabel?: string
  } = $props()
</script>

<span class="input-control" data-trailing={loading || trailing !== undefined || undefined} data-loading={loading || undefined}>
  {@render children()}
  {#if trailing !== undefined}
    {@render trailing()}
  {/if}
  {#if loading}
    <span class="input-control-status" role="status">
      <span class="spinner" aria-hidden="true"></span>
      <span class="visually-hidden">{loadingLabel}</span>
    </span>
  {/if}
</span>
