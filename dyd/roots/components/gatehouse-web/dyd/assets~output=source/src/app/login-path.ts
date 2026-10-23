export function loginPath(returnTo: string): string {
  return `/app/login?next=${encodeURIComponent(returnTo)}`
}
