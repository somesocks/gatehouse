<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    title,
    description,
    children,
    compact = false,
    level = 2,
  }: {
    title: string
    description?: string
    children: Snippet
    compact?: boolean
    level?: number
  } = $props()
</script>

<fieldset
  class={compact
    ? "grid min-w-0 gap-5 border-b p-6 last:border-b-0"
    : "grid min-w-0 gap-5 border-b p-6 last:border-b-0 lg:grid-cols-[minmax(0,1fr)_minmax(0,3fr)] lg:gap-8"}
>
  <legend class="sr-only">{title}</legend>
  <div class="min-w-0 space-y-2">
    <svelte:element
      this={`h${Math.min(6, Math.max(1, level))}`}
      class="text-sm font-semibold">{title}</svelte:element
    >
    {#if description}<p class="text-muted-foreground text-sm leading-relaxed">
        {description}
      </p>{/if}
  </div>
  <div
    class="grid min-w-0 content-start items-start gap-5 xl:grid-cols-2 [&>*:only-child]:col-span-full [&>*:has(textarea)]:col-span-full [&>*:has(fieldset)]:col-span-full [&>.grid]:col-span-full"
  >
    {@render children()}
  </div>
</fieldset>
