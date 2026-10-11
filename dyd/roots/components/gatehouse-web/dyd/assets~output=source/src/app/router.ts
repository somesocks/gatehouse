import { legacySystemRedirect, parseRoute, type Route } from "../route";

export type Navigate = (path: string, replace?: boolean) => void;

export function createRouter(onRoute: (route: Route) => void): {
  navigate: Navigate;
  resolve: () => void;
  start: () => () => void;
} {
  function resolve(): void {
    let url = new URL(window.location.href);
    const legacyPath = legacySystemRedirect(url);
    if (legacyPath !== null) {
      window.history.replaceState(null, "", legacyPath);
      url = new URL(window.location.href);
    }
    onRoute(parseRoute(url));
  }

  function navigate(path: string, replace = false): void {
    window.history[replace ? "replaceState" : "pushState"](null, "", path);
    resolve();
  }

  function start(): () => void {
    window.addEventListener("popstate", resolve);
    return () => window.removeEventListener("popstate", resolve);
  }

  return { navigate, resolve, start };
}
