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
  class={mode === "inline"
    ? "h-8 min-h-8 max-h-40 py-1 text-sm leading-5 resize-none"
    : undefined}
  {...rest}
/>
