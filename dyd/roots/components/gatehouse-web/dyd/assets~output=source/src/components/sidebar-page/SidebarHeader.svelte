<script lang="ts">
  import { X } from "@lucide/svelte"

  let { title }: { title?: string } = $props()
</script>

<header class="sidebar-page-sidebar-header">
  {#if title !== undefined}<h2>{title}</h2>{/if}
  <form class="sidebar-page-sidebar-close" method="dialog">
    <button aria-label="Close navigation" class="icon inline small" type="submit"
      ><X size={16} aria-hidden="true" /></button
    >
  </form>
</header>
