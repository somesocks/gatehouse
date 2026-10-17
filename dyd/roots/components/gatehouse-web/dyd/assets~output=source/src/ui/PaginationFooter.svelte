<script lang="ts">
  import Button from "./Button.svelte"
  let {
    count,
    noun = "records",
    previous = false,
    next = false,
    loading = false,
    onprevious,
    onnext,
    showCount = true,
  }: {
    count: number
    noun?: string
    previous?: boolean
    next?: boolean
    loading?: boolean
    onprevious?: () => void
    onnext?: () => void
    showCount?: boolean
  } = $props()
</script>

<footer
  class={showCount
    ? "flex flex-wrap items-center justify-between gap-4"
    : "flex items-center justify-end gap-2"}
>
  {#if showCount}<p class="text-muted-foreground text-sm">
      Showing {count}
      {count === 1 ? noun.replace(/s$/, "") : noun} on this page
    </p>{/if}
  <div class="flex gap-2">
    <Button
      variant="outline"
      size="sm"
      disabled={!previous || loading}
      onclick={onprevious}>Previous</Button
    >
    <Button
      variant="outline"
      size="sm"
      disabled={!next || loading}
      onclick={onnext}>{loading ? "Loading…" : "Next"}</Button
    >
  </div>
</footer>
