<script lang="ts">
  import type { ComponentProps } from "svelte"
  import Link from "./Link.svelte"
  import { routerLinkPath } from "../app/router-link"
  import { useRuntime } from "../app/runtime.svelte"
  let {
    href,
    children,
    onclick,
    ...rest
  }: ComponentProps<typeof Link> & { href: string } = $props()
  const runtime = useRuntime()
  function navigate(event: MouseEvent) {
    onclick?.(event as MouseEvent & { currentTarget: HTMLAnchorElement })
    if (event.defaultPrevented) return
    const path = routerLinkPath(event, {
      href,
      target: rest.target ?? undefined,
      download: rest.download,
      origin: window.location.origin,
      currentURL: window.location.href,
    })
    if (path !== null) {
      event.preventDefault()
      runtime.navigate(path)
    }
  }
</script>

<Link {href} {...rest} onclick={navigate}>{@render children?.()}</Link>
