<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    title = "Configuration",
    children,
  }: { title?: string; children: Snippet } = $props()
</script>

<section class="overflow-hidden rounded-lg border bg-card text-card-foreground">
  <h2 class="border-b bg-muted/20 px-6 py-4 text-sm font-semibold">{title}</h2>
  <dl class="divide-y px-6">{@render children()}</dl>
</section>
