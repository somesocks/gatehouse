<script lang="ts">
  import { Button } from "../lib/vendor/shadcn/button"
  import type { ComponentProps } from "svelte"
  let {
    ref = $bindable(),
    ...rest
  }: Omit<ComponentProps<typeof Button>, "class" | "style"> = $props()
</script>

<Button
  bind:ref={
    () => ref ?? null,
    (element) => {
      ref = element ?? undefined
    }
  }
  {...rest}
/>
