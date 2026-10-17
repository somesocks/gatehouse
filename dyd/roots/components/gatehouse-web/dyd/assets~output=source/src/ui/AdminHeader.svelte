<script lang="ts">
  import type { Snippet } from "svelte"
  import Identifier from "./Identifier.svelte"
  import StatusBadge from "./StatusBadge.svelte"
  let {
    title,
    description,
    kind,
    id,
    enabled,
    actions,
    compact = false,
  }: {
    title: string
    description?: string
    kind?: string
    id?: string
    enabled?: boolean
    actions?: Snippet
    compact?: boolean
  } = $props()
</script>

<header
  class={compact ? "flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between" : "flex flex-col gap-4 border-b pb-5 sm:flex-row sm:items-start sm:justify-between"}
>
  <div class="min-w-0 flex-1 space-y-2">
    {#if kind}<p
        class="text-muted-foreground text-xs font-medium tracking-wide uppercase"
      >
        {kind}
      </p>{/if}
    <div class="flex flex-wrap items-center gap-3">
      <h1
        class={compact ? "text-lg font-semibold [overflow-wrap:anywhere]" : "text-2xl font-semibold tracking-tight [overflow-wrap:anywhere]"}
      >
        {title}
      </h1>
      {#if enabled !== undefined}<StatusBadge {enabled} />{/if}
    </div>
    {#if description}<p class="text-muted-foreground text-sm leading-relaxed">
        {description}
      </p>{/if}
    {#if id}<Identifier value={id} />{/if}
  </div>
  {#if actions}<div class="flex shrink-0 items-center gap-2">
      {@render actions()}
    </div>{/if}
</header>
