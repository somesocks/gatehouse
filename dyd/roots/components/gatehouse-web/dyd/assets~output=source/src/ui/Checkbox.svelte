<script lang="ts">
  import { Checkbox } from "../lib/vendor/shadcn/checkbox"
  import type { ComponentProps } from "svelte"
  let {
    checked = $bindable(false),
    ...rest
  }: Omit<ComponentProps<typeof Checkbox>, "class" | "style" | "checked"> & {
    checked?: boolean
  } = $props()
</script>

<Checkbox bind:checked {...rest} />
