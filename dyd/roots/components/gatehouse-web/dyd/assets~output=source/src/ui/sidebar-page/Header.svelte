<script lang="ts">
  import { cn } from "../../lib/utils"
  import type { Snippet } from "svelte"
  let {
    placement = "inline",
    divider = false,
    children,
  }: {
    placement?: "inline" | "floating"
    divider?: boolean
    children: Snippet
  } = $props()
</script>

<header
  class={cn(
    "sticky top-0 z-20 flex h-16 shrink-0 items-center bg-background md:rounded-t-xl",
    divider && "border-b",
  )}
>
  <div class="flex w-full min-w-0 items-center gap-2 px-4">
    {@render children()}
  </div>
</header>
