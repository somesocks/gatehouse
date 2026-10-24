<script lang="ts">
  import Button from "./Button.svelte"
  import Notice from "./Notice.svelte"
  import ResourceDialog from "./ResourceDialog.svelte"
  import * as Dialog from "./dialog"
  let { noun, name, scope = "chat", busy = false, error = "", onCancel, onConfirm }: {
    noun: string
    name: string
    scope?: string
    busy?: boolean
    error?: string
    onCancel: () => void
    onConfirm: () => void
  } = $props()
  let cancelButton = $state<HTMLButtonElement>()
</script>

<ResourceDialog
  open={true}
  title={`Remove ${noun}?`}
  viewKey={`remove-${noun}`}
  compact
  {busy}
  includeClose={false}
  onClose={onCancel}
  initialFocus={() => cancelButton}
>
  {#snippet footer()}
    <Button bind:ref={cancelButton} variant="outline" disabled={busy} onclick={onCancel}>Cancel</Button>
    <Button variant="destructive" disabled={busy} onclick={onConfirm}>{busy ? "Removing…" : "Remove"}</Button>
  {/snippet}
<section class="space-y-4">
  <Dialog.Description class="text-sm break-words">Remove “{name}” from this {scope}?</Dialog.Description>
  {#if error}<Notice variant="destructive" role="alert">{error}</Notice>{/if}
</section>
</ResourceDialog>
