import { clearAccessToken, fetchGatehouse, setAccessToken } from "./api"

export type Claims = {
  principal: {
    ref: { id: string }
    name?: string
  }
  identity: string
}

export async function checkAuthentication(): Promise<Claims | null> {
  const response = await fetchGatehouse("/api/v1/auth/me", {
    credentials: "same-origin",
  })
  if (response.status === 401) {
    return null
  }
  if (!response.ok) {
    throw new Error(`authentication check returned ${response.status}`)
  }
  return (await response.json()) as Claims
}

export async function signIn(
  identity: string,
  password: string,
): Promise<boolean> {
  const response = await fetch("/api/v1/auth/login", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ identity, password }),
  })
  if (response.status === 401) {
    return false
  }
  if (!response.ok) {
    throw new Error(`login returned ${response.status}`)
  }
  const credentials = (await response.json()) as { access_token: string }
  if (typeof credentials.access_token !== "string" || credentials.access_token === "")
    throw new Error("login did not return an access token")
  setAccessToken(credentials.access_token)
  return true
}

export async function signOut(): Promise<void> {
  try {
    await fetchGatehouse("/api/v1/auth/logout", {
      method: "POST",
      credentials: "same-origin",
    })
  } finally {
    clearAccessToken()
  }
}
