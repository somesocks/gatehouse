<script lang="ts">
  import { Badge } from "../lib/vendor/shadcn/badge"
  import type { ComponentProps } from "svelte"
  let {
    variant = "secondary",
    ...rest
  }: Omit<ComponentProps<typeof Badge>, "class" | "style"> = $props()
</script>

<Badge {variant} class="h-auto whitespace-normal" {...rest} />
