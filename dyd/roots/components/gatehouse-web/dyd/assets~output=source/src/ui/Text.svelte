<script lang="ts">
  import { cn } from "../lib/utils"
  import type { Attributes } from "./types"
  let {
    as = "span",
    tone = "default",
    size = "normal",
    emphasis = "normal",
    truncate = false,
    hidden = false,
    children,
    ...rest
  }: Attributes & {
    as?:
      | "span"
      | "p"
      | "small"
      | "strong"
      | "em"
      | "dt"
      | "dd"
      | "legend"
      | "time"
    tone?: "default" | "muted" | "destructive"
    size?: "normal" | "small" | "eyebrow"
    emphasis?: "normal" | "strong"
    truncate?: boolean
    hidden?: boolean
    datetime?: string
  } = $props()
</script>

<svelte:element
  this={as}
  class={cn(
    hidden ? "sr-only" : "break-words",
    tone === "muted" && "text-muted-foreground",
    tone === "destructive" && "text-destructive",
    size === "small" && "text-sm text-muted-foreground",
    size === "eyebrow" &&
      "text-xs font-medium uppercase tracking-wide text-muted-foreground",
    emphasis === "strong" && "font-medium",
    truncate && "truncate",
  )}
  {...rest}>{@render children?.()}</svelte:element
>
