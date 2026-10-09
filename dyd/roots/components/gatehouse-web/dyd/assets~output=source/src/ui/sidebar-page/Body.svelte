<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    element = $bindable(),
    padding = "normal",
    children,
  }: {
    element?: HTMLElement
    padding?: "normal" | "none"
    children: Snippet
  } = $props()
</script>

<section
  bind:this={element}
  class={padding === "none"
    ? "min-w-0 flex-1 p-0"
    : "min-w-0 flex-1 p-4 md:p-6"}
>
  {@render children()}
</section>
