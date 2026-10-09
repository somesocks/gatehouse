<script lang="ts">
  import { Input } from "../lib/vendor/shadcn/command"
  import type { ComponentProps } from "svelte"
  let {
    ref = $bindable(),
    value = $bindable(),
    ...rest
  }: Omit<ComponentProps<typeof Input>, "class" | "style" | "ref"> & {
    ref?: HTMLInputElement
  } = $props()
</script>

<Input
  bind:ref={
    () => ref ?? null,
    (element) => {
      ref = element instanceof HTMLInputElement ? element : undefined
    }
  }
  bind:value={
    () => value ?? "",
    (next) => {
      value = next
    }
  }
  {...rest}
/>
