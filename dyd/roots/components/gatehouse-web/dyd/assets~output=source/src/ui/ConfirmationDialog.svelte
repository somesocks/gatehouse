<script lang="ts">
  import { onDestroy, untrack } from "svelte"
  import * as Dialog from "./dialog"
  import Button from "./Button.svelte"
  import ActionButton from "./ActionButton.svelte"
  import Notice from "./Notice.svelte"
  import type { Confirmation } from "../utils/confirmation.svelte"

  let { confirmation, contextKey }: {
    confirmation: Confirmation
    contextKey: unknown
  } = $props()
  let cancelButton = $state<HTMLButtonElement>()

  $effect.pre(() => {
    contextKey
    untrack(() => confirmation.reset())
  })
  onDestroy(() => confirmation.reset())
</script>

<Dialog.Root
  open={confirmation.state.request !== null}
  onOpenChange={(open) => { if (!open) confirmation.cancel() }}
>
  <Dialog.Content
    size="compact"
    showCloseButton={!confirmation.state.pending}
    onOpenAutoFocus={(event) => {
      event.preventDefault()
      cancelButton?.focus()
    }}
    onEscapeKeydown={(event) => { if (confirmation.state.pending) event.preventDefault() }}
    onInteractOutside={(event) => { if (confirmation.state.pending) event.preventDefault() }}
  >
    <Dialog.Title>{confirmation.state.request?.title ?? "Confirm action"}</Dialog.Title>
    <Dialog.Description>{confirmation.state.request?.description}</Dialog.Description>
    {#if confirmation.state.error}<Notice variant="destructive" role="alert">
      {confirmation.state.error}
    </Notice>{/if}
    <div class="flex flex-wrap items-center justify-end gap-2">
      <Button bind:ref={cancelButton} variant="outline" disabled={confirmation.state.pending} onclick={confirmation.cancel}>Cancel</Button>
      <ActionButton variant="destructive" pending={confirmation.state.pending} onclick={() => void confirmation.confirm()}>
        {confirmation.state.request?.confirmLabel ?? "Confirm"}
      </ActionButton>
    </div>
  </Dialog.Content>
</Dialog.Root>
