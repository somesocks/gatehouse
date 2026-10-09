<script lang="ts">
  import { Title } from "../lib/vendor/shadcn/dialog"
  import type { ComponentProps } from "svelte"
  let {
    hidden = false,
    ...rest
  }: Omit<ComponentProps<typeof Title>, "class" | "style"> & {
    hidden?: boolean
  } = $props()
</script>

<Title class={hidden ? "sr-only" : undefined} {...rest} />
