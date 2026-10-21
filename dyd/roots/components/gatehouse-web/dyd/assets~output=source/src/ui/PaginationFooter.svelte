<script lang="ts">
  import Button from "./Button.svelte"
  let {
    previous = false,
    next = false,
    loading = false,
    onprevious,
    onnext,
  }: {
    count?: number
    noun?: string
    previous?: boolean
    next?: boolean
    loading?: boolean
    onprevious?: () => void
    onnext?: () => void
    showCount?: boolean
  } = $props()
</script>

{#if previous || next}
  <footer class="flex items-center justify-end gap-2">
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
{/if}
