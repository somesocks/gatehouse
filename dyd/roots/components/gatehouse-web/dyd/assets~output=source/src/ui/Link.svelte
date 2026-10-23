<script lang="ts">
  import type { HTMLAnchorAttributes } from "svelte/elements"
  import { buttonVariants } from "../lib/vendor/shadcn/button"
  import { itemVariants } from "../lib/vendor/shadcn/item/item.svelte"
  import { cn } from "../lib/utils"
  let {
    href,
    disabled = false,
    onclick,
    variant = "link",
    size = "default",
    children,
    ...rest
  }: Omit<HTMLAnchorAttributes, "class" | "style"> & {
    disabled?: boolean
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
  const classes = $derived(cn(
    variant === "default" || variant === "outline" || variant === "secondary" || variant === "ghost"
      ? buttonVariants({ variant, size })
      : variant === "record"
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
            : "text-sm font-medium text-primary underline-offset-4 hover:underline",
    disabled && "pointer-events-none opacity-50",
  ))
</script>

<a
  class={classes}
  {...rest}
  href={disabled ? undefined : href}
  role={rest.role ?? (disabled ? "link" : undefined)}
  aria-disabled={disabled || rest["aria-disabled"]}
  tabindex={disabled ? -1 : rest.tabindex}
  onclick={(event) => {
    if (disabled) {
      event.preventDefault()
      return
    }
    onclick?.(event)
  }}
>{@render children?.()}</a>
