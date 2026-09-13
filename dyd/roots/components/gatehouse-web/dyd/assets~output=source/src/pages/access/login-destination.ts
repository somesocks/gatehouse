export function loginDestination(next: string | null, origin: string): string {
  if (next === null) return "/app/"
  let destination: URL
  try {
    destination = new URL(next, origin)
  } catch {
    return "/app/"
  }
  if (
    destination.origin !== origin ||
    !destination.pathname.startsWith("/app/") ||
    destination.pathname === "/app/login" ||
    destination.pathname === "/app/login/"
  )
    return "/app/"
  return destination.pathname + destination.search + destination.hash
}
