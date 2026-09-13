export type RouterLinkClick = Pick<
  MouseEvent,
  "button" | "defaultPrevented" | "metaKey" | "altKey" | "ctrlKey" | "shiftKey"
>

type Options = {
  href: string
  target?: string
  download?: string | boolean
  origin: string
  currentURL: string
}

export function routerLinkPath(
  event: RouterLinkClick,
  { href, target, download, origin, currentURL }: Options,
): string | null {
  if (
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
