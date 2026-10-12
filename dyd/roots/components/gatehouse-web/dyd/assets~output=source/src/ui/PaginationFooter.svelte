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
  }: {
    count: number
    noun?: string
    previous?: boolean
    next?: boolean
    loading?: boolean
    onprevious?: () => void
    onnext?: () => void
  } = $props()
</script>

<footer class="flex flex-wrap items-center justify-between gap-4">
  <p class="text-muted-foreground text-sm">
    Showing {count}
    {count === 1 ? noun.replace(/s$/, "") : noun} on this page
  </p>
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
