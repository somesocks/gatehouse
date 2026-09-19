<script lang="ts">
  import type { Snippet } from "svelte"
  let {
    element = $bindable<HTMLElement | undefined>(undefined),
    children,
  }: { element?: HTMLElement; children: Snippet } = $props()
</script>

<section bind:this={element} class="page-body">
  {@render children()}
</section>
