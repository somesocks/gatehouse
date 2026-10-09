<script lang="ts">
  import { cn } from "../lib/utils"
  import { gaps, type Gap, type Attributes } from "./types"
  let {
    as = "div",
    gap = "normal",
    grow = false,
    tone,
    indent = false,
    ref = $bindable(),
    children,
    ...rest
  }: Attributes & {
    as?:
      | "div"
      | "span"
      | "section"
      | "article"
      | "header"
      | "footer"
      | "main"
      | "aside"
      | "hgroup"
      | "dl"
      | "ul"
      | "ol"
      | "li"
    gap?: Gap
    grow?: boolean
    tone?: "muted"
    indent?: boolean
    ref?: HTMLElement
  } = $props()
</script>

<svelte:element
  this={as}
  bind:this={ref}
  class={cn(
    "flex min-w-0 flex-col",
    gaps[gap],
    grow && "flex-1",
    tone === "muted" && "text-muted-foreground",
    indent && "border-l pl-4",
  )}
  {...rest}
>
  {@render children?.()}
</svelte:element>
