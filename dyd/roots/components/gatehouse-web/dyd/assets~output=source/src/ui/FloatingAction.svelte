<script lang="ts">
  import { Button } from "../lib/vendor/shadcn/button"
  import type { ComponentProps } from "svelte"
  let {
    ...rest
  }: Omit<ComponentProps<typeof Button>, "class" | "style"> = $props()
</script>

<Button class="absolute right-4 bottom-4 z-10 shadow-sm md:right-6" {...rest} />
