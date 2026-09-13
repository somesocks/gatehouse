import { parseRoute, type Route } from "../route"

export type Navigate = (path: string, replace?: boolean) => void

export function createRouter(onRoute: (route: Route) => void): {
  navigate: Navigate
  resolve: () => void
  start: () => () => void
} {
  function resolve(): void {
    onRoute(parseRoute(new URL(window.location.href)))
  }

  function navigate(path: string, replace = false): void {
    window.history[replace ? "replaceState" : "pushState"](null, "", path)
    resolve()
  }

  function start(): () => void {
    window.addEventListener("popstate", resolve)
    return () => window.removeEventListener("popstate", resolve)
  }

  return { navigate, resolve, start }
}
