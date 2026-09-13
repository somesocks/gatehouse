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
      pageState.floatingFooterHeight = 0
      return
    }
    const update = () =>
      (pageState.floatingFooterHeight =
        element?.getBoundingClientRect().height ?? 0)
    update()
    const observer = new ResizeObserver(update)
    observer.observe(element)
    return () => {
      observer.disconnect()
      pageState.floatingFooterHeight = 0
    }
  })
</script>

<footer
  bind:this={element}
  class:sidebar-page-footer--floating={placement === "floating"}
  class="sidebar-page-footer"
>
  <div class="sidebar-page-footer-content">{@render children()}</div>
</footer>
