// Login bearers are kept only in this page's memory. Embedded Gatehouse pages
// can still use the API when third-party cookies are unavailable.
let accessToken: string | null = null

export function setAccessToken(token: string): void {
  accessToken = token
}

export function clearAccessToken(): void {
  accessToken = null
}

export function fetchGatehouse(
  input: RequestInfo | URL,
  init: RequestInit = {},
): Promise<Response> {
  if (accessToken === null || typeof input !== "string" || !input.startsWith("/api/v1/"))
    return globalThis.fetch(input, init)
  const headers = new Headers(init.headers)
  headers.set("Authorization", `Bearer ${accessToken}`)
  return globalThis.fetch(input, { ...init, headers })
}
