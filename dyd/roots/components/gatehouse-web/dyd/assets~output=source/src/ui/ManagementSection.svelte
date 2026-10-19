<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    title,
    description,
    actions,
    toolbar,
    footer,
    children,
    loading = false,
  }: {
    title: string
    description?: string
    actions?: Snippet
    toolbar?: Snippet
    footer?: Snippet
    children: Snippet
    loading?: boolean
  } = $props()
  const uid = $props.id()
</script>

<section
  aria-labelledby={uid}
  aria-busy={loading}
  class="min-w-0 overflow-hidden rounded-lg border bg-card text-card-foreground"
>
  <header class="flex flex-wrap items-center justify-between gap-3 px-5 py-4">
    <h2 id={uid} class="text-sm font-semibold">{title}</h2>
    {#if actions}<div class="flex shrink-0 items-center gap-2">
        {@render actions()}
      </div>{/if}
  </header>
  {#if description}<p
      class="text-muted-foreground px-5 pb-4 text-sm leading-relaxed"
    >
      {description}
    </p>{/if}
  {#if toolbar}<div class="border-y px-5 py-3">{@render toolbar()}</div>{/if}
  {@render children()}
  {#if footer}<footer class="space-y-3 border-t px-5 py-3">
      {@render footer()}
    </footer>{/if}
</section>
