<script lang="ts">
  import { Field } from "../lib/vendor/shadcn/field"
  import type { ComponentProps } from "svelte"
  import { cn } from "../lib/utils"
  let {
    grow = false,
    span,
    ...rest
  }: Omit<ComponentProps<typeof Field>, "class" | "style"> & {
    grow?: boolean
    span?: "full"
  } = $props()
</script>

<Field
  class={cn(grow && "flex-1", span === "full" && "col-span-full")}
  {...rest}
/>
