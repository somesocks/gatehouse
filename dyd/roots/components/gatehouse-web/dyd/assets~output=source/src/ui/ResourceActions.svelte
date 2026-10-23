<script lang="ts">
  import type { Snippet } from "svelte"
  import { resourceDialog } from "./resource-dialog-context"

  let { children, includeClose = true, busy = false }: {
    children: Snippet
    includeClose?: boolean
    busy?: boolean
  } = $props()
  const dialog = resourceDialog()
  const owner = Symbol("resource-actions")
  $effect(() => {
    if (!dialog) return
    dialog.footer = { owner, children, includeClose, busy }
    return () => {
      if (dialog.footer?.owner === owner) dialog.footer = undefined
    }
  })
</script>

{#if !dialog}
  <div class="flex flex-wrap items-center justify-end gap-2">{@render children()}</div>
{/if}
