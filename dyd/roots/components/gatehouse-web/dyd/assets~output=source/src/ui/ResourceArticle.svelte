<script lang="ts">
  import type { Snippet } from "svelte"
  let { title, summary, metadata, children, provenance }: {
    title?: string
    summary?: string
    metadata?: Snippet
    children: Snippet
    provenance?: Snippet
  } = $props()
</script>

<article class="min-w-0 space-y-6 pb-2">
  {#if title || summary || metadata}<header class="min-w-0 space-y-3">
    {#if title}<h2 class="text-2xl font-semibold tracking-tight [overflow-wrap:anywhere]">{title}</h2>{/if}
    {#if summary}<p class="text-muted-foreground text-sm leading-relaxed break-words">{summary}</p>{/if}
    {#if metadata}<div class="text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
      {@render metadata()}
    </div>{/if}
  </header>{/if}
  <div class="min-w-0">{@render children()}</div>
  {#if provenance}<footer class="text-muted-foreground space-y-1 text-sm leading-relaxed">
    {@render provenance()}
  </footer>{/if}
</article>
