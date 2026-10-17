<script lang="ts">
  import type { HTMLAnchorAttributes } from "svelte/elements"
  import { buttonVariants } from "../lib/vendor/shadcn/button"
  import { itemVariants } from "../lib/vendor/shadcn/item/item.svelte"
  import { cn } from "../lib/utils"
  let {
    variant = "link",
    size = "default",
    children,
    ...rest
  }: Omit<HTMLAnchorAttributes, "class" | "style"> & {
    variant?:
      | "link"
      | "default"
      | "outline"
      | "secondary"
      | "ghost"
      | "item"
      | "entry"
      | "card"
       | "navigation"
       | "record"
    size?: "default" | "sm" | "icon"
  } = $props()
</script>

{#if variant === "default" || variant === "outline" || variant === "secondary" || variant === "ghost"}
  <a class={buttonVariants({ variant, size })} {...rest}
    >{@render children?.()}</a
  >
{:else}
  <a
    class={variant === "record"
      ? "flex min-w-0 flex-1 flex-col gap-1 rounded-sm text-sm text-foreground focus-visible:outline-2 focus-visible:outline-ring"
      : variant === "navigation"
      ? "flex min-w-0 items-center gap-2 rounded-md text-sm hover:underline aria-[current=page]:font-medium aria-[current=page]:text-sidebar-foreground"
      : variant === "item" || variant === "entry" || variant === "card"
        ? cn(
            itemVariants({
              variant: variant === "entry" ? "default" : "outline",
            }),
            "flex-col items-stretch",
          )
        : "text-sm font-medium text-primary underline-offset-4 hover:underline"}
    {...rest}>{@render children?.()}</a
  >
{/if}
