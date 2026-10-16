import { fetchGatehouse as fetch } from "./api"

export type SystemGrant = {
  ref: { id: string }
  principal: { id: string }
  enabled: boolean
  revision: number
}

export type SystemPrincipalIdentity = {
  id: string
  key: string
  enabled: boolean
  revision: number
}

export type SystemPrincipal = {
  id: string
  alias?: string
  name?: string
  enabled: boolean
  revision: number
  identities: SystemPrincipalIdentity[]
  identity_count?: number
}

export type SystemKeychain = { id: string; version: number }
export type CodexLoginAttempt = {
  device_auth_id: string
  user_code: string
  verification_uri: string
  expires_at: string
  poll_after_ms: number
}
export type CodexLoginSuccess = {
  status: "succeeded"
  account: { id: string; email?: string }
  expires_at?: string
  provider: SystemAgentProvider
}
export type CodexLoginBinding = { provider: string; expected_revision: number }
export type CodexLoginPoll =
  | { status: "pending"; poll_after_ms: number }
  | CodexLoginSuccess

export function startCodexLogin(
  binding: CodexLoginBinding,
  signal?: AbortSignal,
): Promise<Response> {
  return systemAdministration(
    "agent-providers/openai-codex/login/start",
    "POST",
    binding,
    signal,
  )
}

export function pollCodexLogin(
  attempt: Pick<CodexLoginAttempt, "device_auth_id" | "user_code">,
  binding: CodexLoginBinding,
  signal?: AbortSignal,
): Promise<Response> {
  return systemAdministration(
    "agent-providers/openai-codex/login/poll",
    "POST",
    {
      ...binding,
      device_auth_id: attempt.device_auth_id,
      user_code: attempt.user_code,
    },
    signal,
  )
}

export type SystemAgentProvider = {
  id: string
  alias: string
  revision: number
  protocol: string
  endpoint?: string
  keychain?: SystemKeychain
  credential_configured: boolean
  enabled: boolean
  account?: { id: string; email?: string; expires_at?: string }
}

export function fetchAgentProvider(
  providerID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return systemAdministration(
    `agent-providers/${encodeURIComponent(providerID)}`,
    "GET",
    undefined,
    signal,
  )
}

export function disconnectAgentProvider(
  providerID: string,
  revision: number,
  signal?: AbortSignal,
): Promise<Response> {
  return systemAdministration(
    `agent-providers/${encodeURIComponent(providerID)}/disconnect`,
    "POST",
    { expected_revision: revision },
    signal,
  )
}
export type SystemAgentModel = {
  id: string
  alias: string
  revision: number
  provider: string
  provider_alias?: string
  model: string
  parameters: string
  compaction: string
  max_turns: number
  max_output_tokens: number
  result_inline_bytes: number
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
export type SystemWorkspace = {
  id: string
  alias?: string
  name?: string
  enabled: boolean
}
export type SystemWorkspaceSearchResponse = {
  workspaces: SystemWorkspace[]
  next_cursor?: string
}
export type SystemWorkspacePrincipal = {
  id: string
  alias?: string
  name?: string
  enabled: boolean
  roles: WorkspaceRole[]
  grants: SystemWorkspacePrincipalGrant[]
}
export type SystemWorkspacePrincipalOption = {
  id: string
  alias?: string
  name?: string
  enabled: boolean
}
export type SystemWorkspacePrincipalSearchResponse = {
  principals: SystemWorkspacePrincipal[]
  next_cursor?: string
}
export type SystemWorkspacePrincipalOptionSearchResponse = {
  principals: SystemWorkspacePrincipalOption[]
  next_cursor?: string
}
export type SystemWorkspacePrincipalGrant = {
  id: string
  workspace: string
  principal: string
  role: WorkspaceRole
  enabled: boolean
  revision: number
}
export type WorkspaceRole = "member" | "contributor" | "manager"
export type TokenBucket = {
  minimum_balance: number
  maximum_balance: number
  refill_per_minute: number
}
export type WorkspaceAgentRateLimits = {
  workspace_input?: TokenBucket
  workspace_output?: TokenBucket
  user_input?: TokenBucket
  user_output?: TokenBucket
}
export type SystemWorkspaceAgent = {
  id: string
  workspace: string
  alias: string
  model: string
  revision: number
  model_alias?: string
  label?: string
  system_prompt?: string
  prelude?: string
  rate_limits?: WorkspaceAgentRateLimits
  default: boolean
  enabled: boolean
}
export type SystemWorkspaceAgentSearchResponse = {
  agents: SystemWorkspaceAgent[]
  next_cursor?: string
}
export type SystemWorkspaceStorageProvider = {
  workspace: string
  provider: string
  provider_alias?: string
  revision: number
  priority: number
  enabled: boolean
}
export type SystemWorkspaceStorageProviderSearchResponse = {
  bindings: SystemWorkspaceStorageProvider[]
  next_cursor?: string
}

export async function systemAdministration(
  path: string,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(`/api/v1/system/${path}`, {
    method,
    credentials: "same-origin",
    headers:
      body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
    ...(signal === undefined ? {} : { signal }),
  })
}

export async function fetchSystemWorkspaces(
  name: string,
  cursor: string,
  signal?: AbortSignal,
): Promise<Response> {
  const parameters = new URLSearchParams({ limit: "50" })
  if (name.trim() !== "") parameters.set("name", name)
  if (cursor !== "") parameters.set("cursor", cursor)
  return await fetch(`/api/v1/system/workspaces?${parameters}`, {
    credentials: "same-origin",
    ...(signal === undefined ? {} : { signal }),
  })
}

export async function createSystemWorkspace(
  input: {
    alias: string
    name?: string
    enabled: boolean
  },
  signal?: AbortSignal,
): Promise<Response> {
  return await systemAdministration("workspaces", "POST", input, signal)
}

export async function fetchSystemWorkspace(
  workspaceID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await systemAdministration(
    `workspaces/${encodeURIComponent(workspaceID)}`,
    "GET",
    undefined,
    signal,
  )
}

export async function updateSystemWorkspace(
  workspaceID: string,
  input: { name: string; enabled: boolean },
  signal?: AbortSignal,
): Promise<Response> {
  return await systemAdministration(
    `workspaces/${encodeURIComponent(workspaceID)}`,
    "PATCH",
    input,
    signal,
  )
}

export async function fetchSystemWorkspacePrincipals(
  workspaceID: string,
  name: string,
  cursor: string,
  signal?: AbortSignal,
): Promise<Response> {
  const parameters = new URLSearchParams({ limit: "50" })
  if (name.trim() !== "") parameters.set("name", name.trim())
  if (cursor !== "") parameters.set("cursor", cursor)
  return await systemAdministration(
    `workspaces/${encodeURIComponent(workspaceID)}/principals?${parameters}`,
    "GET",
    undefined,
    signal,
  )
}

export async function fetchSystemWorkspacePrincipalOptions(
  workspaceID: string,
  name: string,
  cursor: string,
  signal?: AbortSignal,
): Promise<Response> {
  const parameters = new URLSearchParams({ limit: "50" })
  if (name.trim() !== "") parameters.set("name", name.trim())
  if (cursor !== "") parameters.set("cursor", cursor)
  return await systemAdministration(
    `workspaces/${encodeURIComponent(workspaceID)}/principal-options?${parameters}`,
    "GET",
    undefined,
    signal,
  )
}

export async function createSystemWorkspacePrincipalGrant(
  workspaceID: string,
  principal: string,
  role: WorkspaceRole,
  signal?: AbortSignal,
): Promise<Response> {
  return await systemAdministration(
    `workspaces/${encodeURIComponent(workspaceID)}/principals`,
    "POST",
    { principal, role },
    signal,
  )
}

export async function removeSystemWorkspacePrincipalGrants(
  workspaceID: string,
  principalID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await systemAdministration(
    `workspaces/${encodeURIComponent(workspaceID)}/principals/${encodeURIComponent(principalID)}`,
    "DELETE",
    undefined,
    signal,
  )
}

export async function updateSystemWorkspaceGrantRole(
  workspaceID: string,
  grantID: string,
  role: WorkspaceRole,
  expectedRevision: number,
  signal?: AbortSignal,
): Promise<Response> {
  return await systemAdministration(
    `workspaces/${encodeURIComponent(workspaceID)}/grants/${encodeURIComponent(grantID)}`,
    "PATCH",
    { role, expected_revision: expectedRevision },
    signal,
  )
}

export async function fetchSystemWorkspaceAgents(
  workspaceID: string,
  name: string,
  cursor: string,
  signal?: AbortSignal,
): Promise<Response> {
  const parameters = new URLSearchParams({
    workspace: workspaceID,
    limit: "50",
  })
  if (name.trim() !== "") parameters.set("name", name.trim())
  if (cursor !== "") parameters.set("cursor", cursor)
  return await systemAdministration(
    `workspace-agents?${parameters}`,
    "GET",
    undefined,
    signal,
  )
}

export async function fetchSystemWorkspaceStorageBindings(
  workspaceID: string,
  name: string,
  cursor: string,
  signal?: AbortSignal,
): Promise<Response> {
  const parameters = new URLSearchParams({
    workspace: workspaceID,
    limit: "50",
  })
  if (name.trim() !== "") parameters.set("name", name.trim())
  if (cursor !== "") parameters.set("cursor", cursor)
  return await systemAdministration(
    `workspace-storage-providers?${parameters}`,
    "GET",
    undefined,
    signal,
  )
}

export async function fetchSystemGrants(
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch("/api/v1/system/grants", {
    credentials: "same-origin",
    signal,
  })
}

export async function searchSystemAgentProviders(
  name: string,
  cursor = "",
  status = "",
  signal?: AbortSignal,
): Promise<Response> {
  const query = new URLSearchParams({ limit: "50" })
  if (name.trim()) query.set("name", name.trim())
  if (cursor) query.set("cursor", cursor)
  if (status) query.set("status", status)
  return systemAdministration(
    `agent-providers?${query}`,
    "GET",
    undefined,
    signal,
  )
}
export async function searchSystemAgentModels(
  name: string,
  cursor = "",
  status = "",
  signal?: AbortSignal,
): Promise<Response> {
  const query = new URLSearchParams({ limit: "50" })
  if (name.trim()) query.set("name", name.trim())
  if (cursor) query.set("cursor", cursor)
  if (status) query.set("status", status)
  return systemAdministration(`agent-models?${query}`, "GET", undefined, signal)
}
export async function searchSystemStorageProviders(
  name: string,
  cursor = "",
  status = "",
  signal?: AbortSignal,
): Promise<Response> {
  const query = new URLSearchParams({ limit: "50" })
  if (name.trim()) query.set("name", name.trim())
  if (cursor) query.set("cursor", cursor)
  if (status) query.set("status", status)
  return systemAdministration(
    `storage-providers?${query}`,
    "GET",
    undefined,
    signal,
  )
}

export async function fetchSystemPrincipals(): Promise<Response> {
  return await fetch("/api/v1/system/principals", {
    credentials: "same-origin",
  })
}

export async function searchSystemPrincipals(
  name: string,
  cursor = "",
  status = "",
  signal?: AbortSignal,
): Promise<Response> {
  const query = new URLSearchParams({ limit: "50" })
  if (name.trim()) query.set("name", name.trim())
  if (cursor) query.set("cursor", cursor)
  if (status) query.set("status", status)
  return systemAdministration(`principals?${query}`, "GET", undefined, signal)
}
export async function searchSystemGrants(
  name: string,
  cursor = "",
  status = "",
  signal?: AbortSignal,
): Promise<Response> {
  const query = new URLSearchParams({ limit: "50" })
  if (name.trim()) query.set("name", name.trim())
  if (cursor) query.set("cursor", cursor)
  if (status) query.set("status", status)
  return systemAdministration(`grants?${query}`, "GET", undefined, signal)
}

export async function createSystemGrant(
  principal: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch("/api/v1/system/grants", {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ principal }),
    signal,
  })
}

export async function updateSystemGrant(
  id: string,
  enabled: boolean,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(`/api/v1/system/grants/${encodeURIComponent(id)}`, {
    method: "PATCH",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
    signal,
  })
}

export async function updateSystemPrincipal(
  id: string,
  enabled: boolean,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(`/api/v1/system/principals/${encodeURIComponent(id)}`, {
    method: "PATCH",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
    signal,
  })
}
