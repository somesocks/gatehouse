<script lang="ts">
  import type { ComponentProps } from "svelte"
  import Link from "./Link.svelte"
  import { followRouterLink } from "../app/router-link"
  import { useRuntime } from "../app/runtime.svelte"
  let {
    href,
    children,
    onclick,
    disabled = false,
    replace = false,
    onNavigate,
    ...rest
  }: ComponentProps<typeof Link> & {
    href: string
    replace?: boolean
    onNavigate?: () => void
  } = $props()
  const runtime = useRuntime()
  function navigate(event: MouseEvent) {
    onclick?.(event as MouseEvent & { currentTarget: HTMLAnchorElement })
    if (event.defaultPrevented) return
    followRouterLink(
      event,
      {
        href,
        disabled,
        replace,
        target: rest.target ?? undefined,
        download: rest.download,
        origin: window.location.origin,
        currentURL: window.location.href,
      },
      runtime.navigate,
      onNavigate,
    )
  }
</script>

<Link {href} {disabled} {...rest} onclick={navigate}>{@render children?.()}</Link>
