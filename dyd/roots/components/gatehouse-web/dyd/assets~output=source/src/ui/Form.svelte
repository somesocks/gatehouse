<script lang="ts">
  import type { HTMLFormAttributes } from "svelte/elements"
  import { cn } from "../lib/utils"
  let {
    ref = $bindable(),
    layout = "stack",
    surface = false,
    compact = false,
    children,
    ...rest
  }: Omit<HTMLFormAttributes, "class" | "style"> & {
    ref?: HTMLFormElement
    layout?: "stack" | "row" | "search"
    surface?: boolean
    compact?: boolean
  } = $props()
</script>

<form
  bind:this={ref}
  class={cn(
    layout === "stack"
      ? surface
        ? "flex min-w-0 flex-col"
        : compact ? "flex min-w-0 flex-col gap-4" : "flex min-w-0 flex-col gap-6"
      : "flex flex-wrap items-end gap-3",
    surface &&
      "w-full rounded-lg border bg-card [&>[role=alert]]:m-6 [&>[role=alert]]:w-auto [&_input:not([type=checkbox])]:h-10 [&_[data-slot=native-select]]:h-10",
  )}
  {...rest}
>
  {@render children?.()}
</form>
