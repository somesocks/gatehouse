<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    placement = "inline",
    children,
  }: { placement?: "inline" | "floating"; children: Snippet } = $props()
</script>

<header
  class={`page-header cluster${placement === "floating" ? " surface" : ""}`}
  data-position={placement}
>
  {@render children()}
</header>
