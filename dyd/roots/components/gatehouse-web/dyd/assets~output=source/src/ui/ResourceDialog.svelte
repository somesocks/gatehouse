<script lang="ts">
  import type { Snippet } from "svelte"
  import { tick, untrack } from "svelte"
  import * as Dialog from "./dialog"
  import Button from "./Button.svelte"
  import type { ResourceDialogOptions } from "./resource-dialog"

  let {
    open,
    title,
    viewKey,
    pageKey,
    expanded = false,
    compact = false,
    busy = false,
    dismissible = true,
    includeClose = true,
    search,
    footer,
    initialFocus,
    onClose,
    onClosed,
    children,
  }: ResourceDialogOptions & { children: Snippet } = $props()
  let headerElement = $state<HTMLElement>()
  let bodyElement = $state<HTMLElement>()
  let previousView: string | undefined
  const scrollPositions = new Map<string, number>()
  let previousPage: { view: string; page?: string } | undefined
  const inactive = $derived(busy || !open || !dismissible)

  function close() {
    if (!inactive) onClose()
  }

  function dismiss(event: Event) {
    // The owner may return to a parent view without closing this dialog shell.
    // Keep the primitive open until its controlled `open` prop actually changes.
    event.preventDefault()
    close()
  }

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

  $effect(() => {
    const view = viewKey
    const page = pageKey
    if (previousPage?.view === view && previousPage.page !== page)
      bodyElement?.scrollTo({ top: 0 })
    previousPage = { view, page }
  })
</script>

<Dialog.Root
  {open}
  onOpenChange={(value) => {
    if (!value) close()
  }}
  onOpenChangeComplete={(value) => { if (!value && !open) onClosed?.() }}
>
  <Dialog.Content
    size={expanded
      ? "resource-wide"
      : compact
        ? "resource-compact"
        : "resource"}
    showCloseButton={false}
    onOpenAutoFocus={(event) => {
      const target = initialFocus?.() ?? bodyElement?.querySelector<HTMLInputElement>('input[type="search"]')
      if (target) {
        event.preventDefault()
        target.focus({ preventScroll: true })
      }
    }}
    onEscapeKeydown={dismiss}
    onInteractOutside={dismiss}
  >
    <header
      bind:this={headerElement}
      inert={!open}
      tabindex="-1"
      class="flex shrink-0 items-start gap-2 border-b px-3 py-3 outline-none"
    >
      <Dialog.Title class="min-w-0 flex-1 self-center [overflow-wrap:anywhere]">{title}</Dialog.Title>
    </header>
    <div
      bind:this={bodyElement}
      data-slot="resource-dialog-body"
      inert={!open}
      class="min-h-0 flex-auto overflow-y-auto overscroll-contain px-3 py-4"
    >
      {#if search}<div class="pb-4">{@render search()}</div>{/if}
      {@render children()}
    </div>
    <footer
      inert={!open}
      class="flex shrink-0 flex-wrap items-center justify-end gap-2 border-t px-3 py-3"
    >
      {#if includeClose}
        <Button variant="outline" disabled={inactive} onclick={close}
          >Close</Button
        >
      {/if}
      {#if footer}{@render footer()}{/if}
    </footer>
  </Dialog.Content>
</Dialog.Root>
