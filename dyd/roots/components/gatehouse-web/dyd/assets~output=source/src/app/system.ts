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

export type SystemKeychain = { id: string; version: number }
export type SystemAgentProvider = {
  id: string
  alias: string
  revision: number
  protocol: string
  base_url?: string
  keychain?: SystemKeychain
  credential_configured: boolean
  enabled: boolean
}
export type SystemAgentModel = {
  id: string
  alias: string
  revision: number
  provider: string
  model: string
  parameters: string
  compaction: string
  max_turns: number
  max_output_tokens: number
  enabled: boolean
}
export type SystemStorageProvider = {
  id: string
  alias: string
  revision: number
  protocol: string
  endpoint?: string
  region?: string
  bucket?: string
  access_key_id?: string
  keychain?: SystemKeychain
  credential_configured: boolean
  enabled: boolean
}
export type SystemWorkspaceAgent = {
  id: string
  workspace: string
  alias: string
  model: string
  revision: number
	label?: string
	system_prompt?: string
	prelude?: string
	default: boolean
	enabled: boolean
}
export type SystemWorkspaceStorageProvider = {
  workspace: string
  provider: string
  revision: number
  priority: number
  enabled: boolean
}

export async function systemAdministration(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<Response> {
  return await fetch(`/api/v1/system/${path}`, {
    method,
    credentials: "same-origin",
    headers:
      body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}

export async function fetchSystemGrants(): Promise<Response> {
  return await fetch("/api/v1/system/grants", { credentials: "same-origin" })
}

export async function fetchSystemPrincipals(): Promise<Response> {
  return await fetch("/api/v1/system/principals", {
    credentials: "same-origin",
  })
}

export async function createSystemGrant(principal: string): Promise<Response> {
  return await fetch("/api/v1/system/grants", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ principal }),
  })
}

export async function updateSystemGrant(
  id: string,
  enabled: boolean,
): Promise<Response> {
  return await fetch(`/api/v1/system/grants/${encodeURIComponent(id)}`, {
    method: "PATCH",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  })
}

export async function updateSystemPrincipal(
  id: string,
  enabled: boolean,
): Promise<Response> {
  return await fetch(`/api/v1/system/principals/${encodeURIComponent(id)}`, {
    method: "PATCH",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  })
}
