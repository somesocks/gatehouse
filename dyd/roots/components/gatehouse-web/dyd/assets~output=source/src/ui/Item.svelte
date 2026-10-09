<script lang="ts">
  import { itemVariants } from "../lib/vendor/shadcn/item/item.svelte"
  import type { Attributes } from "./types"
  let {
    interactive = false,
    selected = false,
    disabled = false,
    type = "button",
    children,
    ...rest
  }: Attributes & {
    interactive?: boolean
    selected?: boolean
    disabled?: boolean
    type?: "button" | "submit" | "reset"
  } = $props()
</script>

<svelte:element
  this={interactive ? "button" : "div"}
  type={interactive ? type : undefined}
  class={itemVariants({ variant: selected ? "muted" : "outline" })}
  aria-pressed={interactive ? selected : undefined}
  disabled={interactive ? disabled : undefined}
  data-disabled={disabled || undefined}
  {...rest}>{@render children?.()}</svelte:element
>
