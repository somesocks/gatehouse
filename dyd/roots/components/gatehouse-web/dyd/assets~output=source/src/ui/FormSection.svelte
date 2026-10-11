<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    title,
    description,
    children,
  }: { title: string; description?: string; children: Snippet } = $props()
</script>

<fieldset
  class="grid min-w-0 gap-5 border-b p-6 last:border-b-0 lg:grid-cols-[minmax(0,1fr)_minmax(0,3fr)] lg:gap-8"
>
  <legend class="sr-only">{title}</legend>
  <div class="min-w-0 space-y-2">
    <h2 class="text-sm font-semibold">{title}</h2>
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
