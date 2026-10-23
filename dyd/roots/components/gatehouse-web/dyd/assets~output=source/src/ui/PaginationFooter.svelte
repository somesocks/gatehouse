<script lang="ts">
  import Button from "./Button.svelte"
  import Link from "./Link.svelte"
  let {
    previous = false,
    next = false,
    loading = false,
    onprevious,
    onnext,
    previousHref,
    nextHref,
  }: {
    count?: number
    noun?: string
    previous?: boolean
    next?: boolean
    loading?: boolean
    onprevious?: (event: MouseEvent) => void
    onnext?: (event: MouseEvent) => void
    previousHref?: string
    nextHref?: string
    showCount?: boolean
  } = $props()
</script>

{#if previous || next}
  <footer class="flex items-center justify-end gap-2">
    <div class="flex gap-2">
      {#if previousHref}<Link
          variant="outline"
          size="sm"
          href={previousHref}
          disabled={!previous || loading}
          onclick={onprevious}>Previous</Link
        >{:else}<Button
        variant="outline"
        size="sm"
        disabled={!previous || loading}
        onclick={onprevious}>Previous</Button
      >{/if}
      {#if nextHref}<Link
          variant="outline"
          size="sm"
          href={nextHref}
          disabled={!next || loading}
          onclick={onnext}>{loading ? "Loading…" : "Next"}</Link
        >{:else}<Button
        variant="outline"
        size="sm"
        disabled={!next || loading}
        onclick={onnext}>{loading ? "Loading…" : "Next"}</Button
      >{/if}
    </div>
  </footer>
{/if}
