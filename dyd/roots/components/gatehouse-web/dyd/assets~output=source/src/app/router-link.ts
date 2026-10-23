export type RouterLinkClick = Pick<
  MouseEvent,
  "button" | "defaultPrevented" | "metaKey" | "altKey" | "ctrlKey" | "shiftKey"
>

type Options = {
  href: string
  disabled?: boolean
  target?: string
  download?: string | boolean
  origin: string
  currentURL: string
}

export function routerLinkPath(
  event: RouterLinkClick,
  { href, disabled, target, download, origin, currentURL }: Options,
): string | null {
  if (
    disabled ||
    event.defaultPrevented ||
    event.button !== 0 ||
    event.metaKey ||
    event.altKey ||
    event.ctrlKey ||
    event.shiftKey ||
    download !== undefined ||
    (target !== undefined && target !== "" && target !== "_self")
  ) {
    return null
  }
  const destination = new URL(href, currentURL)
  if (destination.origin !== origin) {
    return null
  }
  return destination.pathname + destination.search + destination.hash
}

export function followRouterLink(
  event: RouterLinkClick & Pick<MouseEvent, "preventDefault">,
  options: Options & { replace?: boolean },
  navigate: (path: string, replace?: boolean) => void,
  onNavigate?: () => void,
): void {
  if (options.disabled) {
    event.preventDefault()
    return
  }
  const path = routerLinkPath(event, options)
  if (path === null) return
  event.preventDefault()
  onNavigate?.()
  navigate(path, options.replace ?? false)
}
