<script lang="ts">
  import { cn } from "../../lib/utils"
  import type { Snippet } from "svelte"
  let {
    placement = "inline",
    divider = false,
    compactBreadcrumb = false,
    children,
  }: {
    placement?: "inline" | "floating"
    divider?: boolean
    compactBreadcrumb?: boolean
    children: Snippet
  } = $props()
</script>

<header
  class={cn(
    "sticky top-0 z-20 flex h-16 shrink-0 items-center bg-background md:rounded-t-xl",
    divider && "border-b",
  )}
>
  <div class={cn(
    "flex w-full min-w-0 items-center gap-2 px-4",
    compactBreadcrumb && "[&>[data-slot=breadcrumb]]:min-w-0 [&>[data-slot=breadcrumb]]:overflow-hidden [&_[data-slot=breadcrumb-list]]:flex-nowrap [&_[data-slot=breadcrumb-list]]:whitespace-nowrap [&_[data-slot=breadcrumb-item]]:min-w-0 [&_[data-slot=breadcrumb-page]]:truncate max-md:[&_[data-slot=breadcrumb-item]:not(:last-child)]:hidden max-md:[&_[data-slot=breadcrumb-separator]]:hidden",
  )}>
    {@render children()}
  </div>
</header>
