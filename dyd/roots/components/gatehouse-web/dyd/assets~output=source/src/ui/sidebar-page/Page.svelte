<script lang="ts">
  import { Inset } from "../../lib/vendor/shadcn/sidebar"
  import type { Snippet } from "svelte"
  let {
    element = $bindable(),
    children,
  }: { element?: HTMLElement | null; children: Snippet } = $props()
</script>

<Inset
  bind:ref={
    () => element ?? null,
    (value) => {
      element = value ?? undefined
    }
  }>{@render children()}</Inset
>
