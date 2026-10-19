<script lang="ts">
  import type { Snippet } from "svelte"
  import { tick, untrack } from "svelte"
  import { ArrowLeft, X } from "@lucide/svelte"
  import * as Dialog from "./dialog"
  import Button from "./Button.svelte"
  import {
    provideResourceDialog,
    type ResourceFooter,
  } from "./resource-dialog-context"

  let {
    open,
    title,
    viewKey,
    expanded = false,
    compact = false,
    busy = false,
    backLabel,
    onBack,
    onClose,
    children,
  }: {
    open: boolean
    title: string
    viewKey: string
    expanded?: boolean
    compact?: boolean
    busy?: boolean
    backLabel?: string
    onBack?: () => void
    onClose: () => void
    children: Snippet
  } = $props()
  let headerElement = $state<HTMLElement>()
  let bodyElement = $state<HTMLElement>()
  let previousView: string | undefined
  const scrollPositions = new Map<string, number>()
  const dialog = $state<{ footer?: ResourceFooter; resetScroll: () => void }>({
    footer: undefined,
    resetScroll: () => bodyElement?.scrollTo({ top: 0 }),
  })
  provideResourceDialog(dialog)

  $effect.pre(() => {
    const view = viewKey
    if (previousView !== undefined && previousView !== view)
      scrollPositions.set(
        previousView,
        untrack(() => bodyElement?.scrollTop ?? 0),
      )
    if (!open) {
      previousView = undefined
      scrollPositions.clear()
      return
    }
    const changed = previousView !== undefined && previousView !== view
    previousView = view
    if (changed)
      void tick().then(() => {
        if (!open || viewKey !== view) return
        bodyElement?.scrollTo({ top: scrollPositions.get(view) ?? 0 })
        headerElement?.focus({ preventScroll: true })
      })
  })
</script>

<Dialog.Root
  {open}
  onOpenChange={(value) => {
    if (!value) onClose()
  }}
>
  <Dialog.Content
    size={expanded
      ? "resource-wide"
      : compact
        ? "resource-compact"
        : "resource"}
    showCloseButton={false}
    onEscapeKeydown={(event) => {
      if (busy) event.preventDefault()
    }}
    onInteractOutside={(event) => {
      if (busy) event.preventDefault()
    }}
  >
    <header
      bind:this={headerElement}
      tabindex="-1"
      class="flex shrink-0 items-center gap-3 border-b px-4 py-3 outline-none"
    >
      {#if onBack}
        <Button
          variant="ghost"
          size="icon"
          aria-label={backLabel ?? "Back"}
          title={backLabel ?? "Back"}
          onclick={onBack}
        >
          <ArrowLeft size={18} aria-hidden="true" />
        </Button>
      {/if}
      <Dialog.Title class="min-w-0 flex-1">{title}</Dialog.Title>
      <Button
        variant="ghost"
        size="icon"
        aria-label={`Close ${title.toLowerCase()}`}
        title="Close"
        disabled={busy}
        onclick={onClose}
      >
        <X size={18} aria-hidden="true" />
      </Button>
    </header>
    <div
      bind:this={bodyElement}
      class="min-h-0 flex-auto overflow-y-auto overscroll-contain p-4"
    >
      {@render children()}
    </div>
    <footer
      class="flex shrink-0 flex-wrap items-center justify-end gap-2 border-t px-4 py-3"
    >
      {#if dialog.footer?.includeClose !== false}
        <Button variant="outline" disabled={busy} onclick={onClose}
          >Close</Button
        >
      {/if}
      {#if dialog.footer}{@render dialog.footer.children()}{/if}
    </footer>
  </Dialog.Content>
</Dialog.Root>
