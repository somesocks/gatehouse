<script lang="ts">
  import { Inset } from "../../lib/vendor/shadcn/sidebar"
  import type { Snippet } from "svelte"
  let {
    element = $bindable(),
    children,
  }: { element?: HTMLElement | null; children: Snippet } = $props()
</script>

<Inset
  class="min-h-0 min-w-0"
  bind:ref={
    () => element ?? null,
    (value) => {
      element = value ?? undefined
    }
  }>{@render children()}</Inset
>
