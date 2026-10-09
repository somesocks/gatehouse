<script lang="ts">
  import type { HTMLFormAttributes } from "svelte/elements"
  import { cn } from "../lib/utils"
  let {
    ref = $bindable(),
    layout = "stack",
    children,
    ...rest
  }: Omit<HTMLFormAttributes, "class" | "style"> & {
    ref?: HTMLFormElement
    layout?: "stack" | "row" | "search"
  } = $props()
</script>

<form
  bind:this={ref}
  class={cn(
    layout === "stack"
      ? "flex min-w-0 flex-col gap-6"
      : "flex flex-wrap items-end gap-3",
  )}
  {...rest}
>
  {@render children?.()}
</form>
