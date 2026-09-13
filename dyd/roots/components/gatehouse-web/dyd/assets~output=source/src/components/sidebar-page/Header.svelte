<script lang="ts">
  import type { Snippet } from "svelte"
  import { useSidebarPage } from "./context"

  let {
    placement = "inline",
    children,
  }: { placement?: "inline" | "floating"; children: Snippet } = $props()
  const pageState = useSidebarPage()
  let element = $state<HTMLElement>()

  $effect(() => {
    if (placement !== "floating" || element === undefined) {
      pageState.floatingHeaderHeight = 0
      return
    }
    const update = () =>
      (pageState.floatingHeaderHeight =
        element?.getBoundingClientRect().height ?? 0)
    update()
    const observer = new ResizeObserver(update)
    observer.observe(element)
    return () => {
      observer.disconnect()
      pageState.floatingHeaderHeight = 0
    }
  })
</script>

<header
  bind:this={element}
  class:sidebar-page-header--floating={placement === "floating"}
  class="sidebar-page-header"
>
  {@render children()}
</header>
