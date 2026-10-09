<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    fluid = false,
    width,
    children,
  }: { fluid?: boolean; width?: "wide" | "form"; children: Snippet } = $props()
</script>

<div
  class={width === "form"
    ? "mx-auto flex w-full max-w-2xl flex-col gap-6"
    : fluid || width === "wide"
      ? "flex w-full min-w-0 flex-col gap-6"
      : "mx-auto flex w-full max-w-5xl flex-col gap-6"}
>
  {@render children()}
</div>
