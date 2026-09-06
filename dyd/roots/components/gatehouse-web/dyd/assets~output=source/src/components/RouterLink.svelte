<script lang="ts">
  import type { Snippet } from "svelte"
  import { routerLinkPath } from "../app/router-link"
  import { useRuntime } from "../app/runtime.svelte"

  let { href, target, rel, download, class: className, children }: { href: string; target?: string; rel?: string; download?: string | boolean; class?: string; children: Snippet } = $props()
  const runtime = useRuntime()

  function navigate(event: MouseEvent): void {
    const path = routerLinkPath(event, { href, target, download, origin: window.location.origin, currentURL: window.location.href })
    if (path === null) {
      return
    }
    event.preventDefault()
    runtime.navigate(path)
  }
</script>

<a {href} {target} {rel} {download} class={className} onclick={navigate}>{@render children()}</a>
