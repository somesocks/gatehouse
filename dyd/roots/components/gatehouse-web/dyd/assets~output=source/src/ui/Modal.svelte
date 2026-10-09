<script lang="ts">
  import * as Dialog from "./dialog"
  import { onMount } from "svelte"
  import type { Snippet } from "svelte"
  import type { DialogHandle } from "./types"
  let {
    modal = $bindable(),
    title = "Dialog",
    labelledBy,
    onClose,
    oncancel,
    header,
    children,
  }: {
    modal?: DialogHandle
    title?: string
    labelledBy?: string
    onClose?: () => void
    oncancel?: (event: Event) => void
    header?: Snippet
    children: Snippet
  } = $props()
  let open = $state(false)
  onMount(() => {
    modal = {
      get open() {
        return open
      },
      showModal() {
        open = true
      },
      close() {
        if (!open) return
        open = false
        onClose?.()
      },
    }
    return () => {
      modal = undefined
    }
  })
  function changed(value: boolean) {
    if (!value) onClose?.()
  }
</script>

<Dialog.Root bind:open onOpenChange={changed}>
  <Dialog.Content
    showCloseButton={!header}
    onEscapeKeydown={oncancel}
    onInteractOutside={oncancel}
  >
    {#if header}<Dialog.Title class="sr-only">{title}</Dialog.Title
      >{:else}<Dialog.Title>{title}</Dialog.Title>{/if}
    {#if header}<div class="flex items-center justify-between gap-4">
        {@render header()}
      </div>{/if}
    {@render children()}
  </Dialog.Content>
</Dialog.Root>
