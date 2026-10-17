<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    title = "Configuration",
    children,
    compact = false,
  }: { title?: string; children: Snippet; compact?: boolean } = $props()
</script>

<section class={compact ? "min-w-0" : "overflow-hidden rounded-lg border bg-card text-card-foreground"}>
  <h2 class={compact ? "sr-only" : "border-b bg-muted/20 px-6 py-4 text-sm font-semibold"}>{title}</h2>
  <dl class={compact ? "divide-y [&>div]:py-2" : "divide-y px-6"}>{@render children()}</dl>
</section>
