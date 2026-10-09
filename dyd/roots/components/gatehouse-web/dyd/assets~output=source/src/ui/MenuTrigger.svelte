<script lang="ts">
  import { Trigger } from "../lib/vendor/shadcn/dropdown-menu"
  import { Button } from "../lib/vendor/shadcn/button"
  import type { ComponentProps } from "svelte"
  let {
    variant = "ghost",
    size = "icon",
    children,
    ...rest
  }: Omit<ComponentProps<typeof Trigger>, "class" | "style" | "child"> & {
    variant?: ComponentProps<typeof Button>["variant"]
    size?: ComponentProps<typeof Button>["size"]
  } = $props()
</script>

<Trigger {...rest}
  >{#snippet child({ props })}<Button {variant} {size} {...props}
      >{@render children?.()}</Button
    >{/snippet}</Trigger
>
