<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    placement = "inline",
    children,
  }: { placement?: "inline" | "floating"; children: Snippet } = $props()
</script>

<header
  class="sticky top-0 z-20 flex min-h-14 flex-wrap items-center gap-3 border-b bg-background px-6 py-3"
>
  {@render children()}
</header>
