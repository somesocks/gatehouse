<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    title,
    description,
    children,
    compact = false,
    dialog = false,
    level = 2,
  }: {
    title: string
    description?: string
    children: Snippet
    compact?: boolean
    dialog?: boolean
    level?: number
  } = $props()
</script>

<fieldset
  class={dialog ? "grid min-w-0 gap-4" : compact
    ? "grid min-w-0 gap-5 border-b p-6 last:border-b-0"
    : "grid min-w-0 gap-5 border-b p-6 last:border-b-0 lg:grid-cols-[minmax(0,1fr)_minmax(0,3fr)] lg:gap-8"}
>
  <legend class="sr-only">{title}</legend>
  {#if !dialog}<div class="min-w-0 space-y-2">
    <svelte:element
      this={`h${Math.min(6, Math.max(1, level))}`}
      class="text-sm font-semibold">{title}</svelte:element
    >
    {#if description}<p class="text-muted-foreground text-sm leading-relaxed">
        {description}
      </p>{/if}
  </div>{/if}
  <div
    class={dialog
      ? "grid min-w-0 grid-cols-1 content-start items-start gap-4"
      : "grid min-w-0 content-start items-start gap-5 xl:grid-cols-2 [&>*:only-child]:col-span-full [&>*:has(textarea)]:col-span-full [&>*:has(fieldset)]:col-span-full [&>.grid]:col-span-full"}
  >
    {@render children()}
  </div>
</fieldset>
