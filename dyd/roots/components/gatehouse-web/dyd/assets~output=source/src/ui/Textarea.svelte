<script lang="ts">
  import { Textarea } from "../lib/vendor/shadcn/textarea"
  import type { ComponentProps } from "svelte"
  let {
    value = $bindable(),
    ref = $bindable(),
    mode = "default",
    ...rest
  }: Omit<ComponentProps<typeof Textarea>, "class" | "style"> & {
    mode?: "default" | "inline"
  } = $props()
</script>

<Textarea
  bind:value
  bind:ref={
    () => ref ?? null,
    (value) => {
      ref = value ?? undefined
    }
  }
  class={mode === "inline" ? "min-h-8 max-h-40 resize-none" : undefined}
  {...rest}
/>
