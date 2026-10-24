<script lang="ts">
  import { Content } from "../lib/vendor/shadcn/dialog"
  import type { ComponentProps } from "svelte"
  import { cn } from "../lib/utils"
  let {
    size = "default",
    showCloseButton = false,
    ...rest
  }: Omit<ComponentProps<typeof Content>, "class" | "style"> & {
    size?:
      | "default"
      | "compact"
      | "selector"
      | "wide"
      | "resource"
      | "resource-wide"
      | "resource-compact"
  } = $props()
</script>

<Content
  {showCloseButton}
  class={cn("transition-none data-open:zoom-in-100! data-closed:zoom-out-100!", size === "resource" ||
  size === "resource-wide" ||
  size === "resource-compact"
    ? `flex max-h-[calc(100dvh-2rem)] w-[calc(100%_-_2rem)] flex-col gap-0 overflow-hidden p-0 ${size === "resource-wide" ? "sm:max-w-5xl" : size === "resource-compact" ? "sm:max-w-lg" : "sm:max-w-3xl"}`
    : size === "selector"
      ? "flex h-[min(44rem,85dvh)] max-h-[85dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-3xl"
      : size === "wide"
        ? "max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-3xl"
        : size === "compact"
          ? "max-h-[calc(100dvh-2rem)] overflow-y-auto"
          : "max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl")}
  {...rest}
/>
