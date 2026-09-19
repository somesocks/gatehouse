<script lang="ts">
  import type { Snippet } from "svelte"
  import { useSidebarPage } from "./context"

  let { children }: { children: Snippet } = $props()
  const sidebarPage = useSidebarPage()
  let drawer = $state<HTMLDialogElement>()

  $effect(() => {
    sidebarPage.drawer = drawer
    return () => {
      if (sidebarPage.drawer === drawer) sidebarPage.drawer = undefined
    }
  })

  function closeOnBackdrop(event: MouseEvent): void {
    const bounds = drawer?.getBoundingClientRect()
    if (bounds !== undefined && event.clientX >= bounds.right) drawer.close()
  }
</script>

<aside class="sidebar-page-sidebar stack">
  {@render children()}
</aside>
<dialog
  bind:this={drawer}
  class="sidebar-page-drawer"
  onclick={closeOnBackdrop}
>
  {@render children()}
</dialog>
