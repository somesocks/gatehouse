<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    placement = "inline",
    surface = true,
    children,
  }: {
    placement?: "inline" | "floating"
    surface?: boolean
    children: Snippet
  } = $props()
</script>

<footer
  class={`page-footer${placement === "floating" && surface ? " surface" : ""}`}
  data-position={placement}
  data-surface={surface ? undefined : "none"}
>
  {@render children()}
</footer>
