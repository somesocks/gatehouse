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
      | "item"
      | "entry"
      | "card"
      | "navigation"
    size?: "default" | "sm"
  } = $props()
</script>

{#if variant === "default" || variant === "outline" || variant === "secondary"}
  <a class={buttonVariants({ variant, size })} {...rest}
    >{@render children?.()}</a
  >
{:else}
  <a
    class={variant === "navigation"
      ? "flex rounded-md px-3 py-2 text-sm hover:bg-accent aria-[current=page]:bg-accent aria-[current=page]:font-medium"
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
