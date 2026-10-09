<script lang="ts">
  import { Button } from "../lib/vendor/shadcn/button"
  import type { ComponentProps } from "svelte"

  let {
    ...rest
  }: Omit<
    ComponentProps<typeof Button>,
    "class" | "style" | "variant" | "size"
  > = $props()
</script>

<Button
  variant="link"
  size="sm"
  class="h-auto min-h-0 border-0 bg-transparent p-0 shadow-none active:translate-y-0"
  {...rest}
/>
