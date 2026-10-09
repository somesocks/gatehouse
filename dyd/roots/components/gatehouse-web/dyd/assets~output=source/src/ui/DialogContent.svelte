<script lang="ts">
  import { Content } from "../lib/vendor/shadcn/dialog"
  import type { ComponentProps } from "svelte"
  let {
    size = "default",
    showCloseButton = false,
    ...rest
  }: Omit<ComponentProps<typeof Content>, "class" | "style"> & {
    size?: "default" | "compact"
  } = $props()
</script>

<Content
  {showCloseButton}
  class={size === "compact"
    ? "max-h-[calc(100dvh-2rem)] overflow-y-auto"
    : "max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl"}
  {...rest}
/>
