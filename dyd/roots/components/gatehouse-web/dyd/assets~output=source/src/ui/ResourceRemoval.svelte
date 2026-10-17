<script lang="ts">
  import Button from "./Button.svelte"
  import Notice from "./Notice.svelte"
  import ResourceActions from "./ResourceActions.svelte"
  let { noun, name, scope = "chat", busy = false, error = "", onCancel, onConfirm }: {
    noun: string
    name: string
    scope?: string
    busy?: boolean
    error?: string
    onCancel: () => void
    onConfirm: () => void
  } = $props()
</script>

<section class="space-y-4">
  <h2 class="text-lg font-semibold">Remove {noun}?</h2>
  <p class="text-sm break-words">Remove “{name}” from this {scope}?</p>
  {#if error}<Notice variant="destructive" role="alert">{error}</Notice>{/if}
</section>
<ResourceActions includeClose={false}>
  <Button variant="outline" disabled={busy} onclick={onCancel}>Cancel</Button>
  <Button variant="destructive" disabled={busy} onclick={onConfirm}>{busy ? "Removing…" : "Remove"}</Button>
</ResourceActions>
