export type SystemGrant = {
  ref: { id: string }
  principal: { id: string }
  enabled: boolean
  revision: number
}

export type SystemPrincipal = {
  id: string
  alias?: string
  name?: string
  enabled: boolean
  revision: number
  identities: { id: string; key: string; enabled: boolean; revision: number }[]
}

export async function fetchSystemGrants(): Promise<Response> {
  return await fetch("/api/v1/system/grants", { credentials: "same-origin" })
}

export async function fetchSystemPrincipals(): Promise<Response> {
  return await fetch("/api/v1/system/principals", { credentials: "same-origin" })
}

export async function createSystemGrant(principal: string): Promise<Response> {
  return await fetch("/api/v1/system/grants", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ principal }) })
}

export async function updateSystemGrant(id: string, enabled: boolean): Promise<Response> {
  return await fetch(`/api/v1/system/grants/${encodeURIComponent(id)}`, { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ enabled }) })
}

export async function updateSystemPrincipal(id: string, enabled: boolean): Promise<Response> {
  return await fetch(`/api/v1/system/principals/${encodeURIComponent(id)}`, { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ enabled }) })
}
