<script lang="ts">
  import { cn } from "../../lib/utils"
  import type { Snippet } from "svelte"
  let {
    element = $bindable(),
    padding = "normal",
    scroll = "page",
    children,
  }: {
    element?: HTMLElement
    padding?: "normal" | "none"
    scroll?: "page" | "contained"
    children: Snippet
  } = $props()
</script>

<section
  bind:this={element}
  class={cn(
    "min-w-0 flex-1",
    scroll === "contained" && "flex min-h-0 flex-col overflow-hidden",
    padding === "none" ? "p-0" : "p-4 md:p-6",
  )}
>
  {@render children()}
</section>
