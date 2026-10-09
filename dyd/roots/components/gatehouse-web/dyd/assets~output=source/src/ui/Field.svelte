<script lang="ts">
  import { Field } from "../lib/vendor/shadcn/field"
  import type { ComponentProps } from "svelte"
  let {
    grow = false,
    ...rest
  }: Omit<ComponentProps<typeof Field>, "class" | "style"> & {
    grow?: boolean
  } = $props()
</script>

<Field class={grow ? "flex-1" : undefined} {...rest} />
