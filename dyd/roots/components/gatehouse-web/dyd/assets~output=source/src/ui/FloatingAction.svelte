<script lang="ts">
  import { Button } from "../lib/vendor/shadcn/button"
  import type { ComponentProps } from "svelte"
  let {
    ...rest
  }: Omit<ComponentProps<typeof Button>, "class" | "style"> = $props()
</script>

<Button class="sticky bottom-24 z-10 self-center" {...rest} />
