<script lang="ts">
  import { cn } from "../lib/utils"
  import { gaps, type Gap, type Attributes } from "./types"
  let {
    as = "div",
    gap = "small",
    justify = "start",
    align = "center",
    wrap = true,
    push = false,
    tone,
    children,
    ...rest
  }: Attributes & {
    as?:
      | "div"
      | "span"
      | "section"
      | "header"
      | "footer"
      | "nav"
      | "p"
      | "small"
      | "ul"
      | "ol"
      | "li"
    gap?: Gap
    justify?: "start" | "center" | "end" | "between"
    align?: "center" | "end" | "start"
    wrap?: boolean
    push?: boolean
    tone?: "muted"
  } = $props()
</script>

<svelte:element
  this={as}
  class={cn(
    "flex min-w-0",
    gaps[gap],
    wrap && "flex-wrap",
    push && "ms-auto",
    {
      start: "justify-start",
      center: "justify-center",
      end: "justify-end",
      between: "justify-between",
    }[justify],
    { center: "items-center", start: "items-start", end: "items-end" }[align],
    tone === "muted" && "text-muted-foreground text-sm",
  )}
  {...rest}
>
  {@render children?.()}
</svelte:element>
