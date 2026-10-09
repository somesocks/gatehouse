<script lang="ts">
  import { Input } from "../lib/vendor/shadcn/input"
  import type { ComponentProps } from "svelte"
  type Props = Omit<
    Extract<ComponentProps<typeof Input>, { type: "file" }>,
    "class" | "style" | "ref" | "type"
  > & { ref?: HTMLInputElement; hidden?: boolean; type?: "file" }
  let {
    ref = $bindable(),
    hidden = false,
    type = "file",
    ...rest
  }: Props = $props()
</script>

<Input
  {type}
  bind:ref={
    () => ref ?? null,
    (value) => {
      ref = value instanceof HTMLInputElement ? value : undefined
    }
  }
  class={hidden ? "sr-only" : undefined}
  {...rest}
/>
