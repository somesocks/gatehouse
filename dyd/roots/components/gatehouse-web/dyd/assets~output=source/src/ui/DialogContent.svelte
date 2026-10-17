<script lang="ts">
  import { Content } from "../lib/vendor/shadcn/dialog"
  import type { ComponentProps } from "svelte"
  let {
    size = "default",
    showCloseButton = false,
    ...rest
  }: Omit<ComponentProps<typeof Content>, "class" | "style"> & {
    size?: "default" | "compact" | "selector" | "wide"
  } = $props()
</script>

<Content
  {showCloseButton}
  class={size === "selector"
    ? "flex h-[min(44rem,85dvh)] max-h-[85dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-3xl"
    : size === "wide"
      ? "max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-3xl"
      : size === "compact"
        ? "max-h-[calc(100dvh-2rem)] overflow-y-auto"
        : "max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl"}
  {...rest}
/>
