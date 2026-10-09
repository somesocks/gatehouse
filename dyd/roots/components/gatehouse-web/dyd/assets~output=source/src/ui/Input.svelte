<script lang="ts">
  import { Input } from "../lib/vendor/shadcn/input"
  import type { ComponentProps } from "svelte"
  type Props = Omit<
    Exclude<ComponentProps<typeof Input>, { type: "file" }>,
    "class" | "style" | "ref"
  > & { ref?: HTMLInputElement }
  let { ref = $bindable(), value = $bindable(), ...rest }: Props = $props()
</script>

<Input
  bind:ref={
    () => ref ?? null,
    (element) => {
      ref = element instanceof HTMLInputElement ? element : undefined
    }
  }
  bind:value
  {...rest}
/>
