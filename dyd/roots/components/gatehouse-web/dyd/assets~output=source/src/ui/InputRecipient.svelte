<script lang="ts">
  import { X } from "@lucide/svelte"
  import Button from "./Button.svelte"
  import { cn } from "../lib/utils"

  let {
    name,
    unavailable = false,
    disabled = false,
    onselect,
    onremove,
  }: {
    name: string
    unavailable?: boolean
    disabled?: boolean
    onselect: () => void
    onremove: () => void
  } = $props()
</script>

<div
  data-state={unavailable ? "unavailable" : "ready"}
  class={cn(
    "flex w-fit max-w-full min-w-0 items-center gap-1 rounded-lg border bg-muted/40 py-0.5 pr-0.5 pl-2",
    unavailable && "border-destructive/40 bg-destructive/5",
  )}
>
  <button
    type="button"
    class="focus-visible:ring-ring min-w-0 max-w-48 truncate rounded-sm text-left text-xs font-medium outline-none hover:underline focus-visible:ring-2 disabled:pointer-events-none disabled:opacity-50"
    title={name}
    aria-haspopup="dialog"
    {disabled}
    onclick={onselect}
  >{name}</button>
  {#if unavailable}<span class="text-destructive text-xs">Unavailable</span>{/if}
  <Button
    variant="ghost"
    size="icon-xs"
    type="button"
    aria-label={`Remove ${name}`}
    title={`Remove ${name}`}
    {disabled}
    onclick={onremove}
  ><X size={12} aria-hidden="true" /></Button>
</div>
