export type SessionSecret = {
  id: string
  description: string
  author: { id: string; name?: string }
  created_at: string
  updated_at: string
}

function sessionSecretsPath(workspaceID: string, sessionID: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/sessions/${encodeURIComponent(sessionID)}/secrets`
}

export async function fetchSessionSecrets(workspaceID: string, sessionID: string): Promise<Response> {
  return await fetch(sessionSecretsPath(workspaceID, sessionID), { credentials: "same-origin" })
}

export async function fetchSessionSecret(workspaceID: string, sessionID: string, secretID: string): Promise<Response> {
  return await fetch(`${sessionSecretsPath(workspaceID, sessionID)}/${encodeURIComponent(secretID)}`, { credentials: "same-origin" })
}

export async function createSessionSecret(workspaceID: string, sessionID: string, input: { description: string; value: string }): Promise<Response> {
  return await fetch(sessionSecretsPath(workspaceID, sessionID), { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) })
}

export async function updateSessionSecret(workspaceID: string, sessionID: string, secretID: string, input: { description: string; value?: string }): Promise<Response> {
  return await fetch(`${sessionSecretsPath(workspaceID, sessionID)}/${encodeURIComponent(secretID)}`, { method: "PATCH", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) })
}

export async function removeSessionSecret(workspaceID: string, sessionID: string, secretID: string): Promise<Response> {
  return await fetch(`${sessionSecretsPath(workspaceID, sessionID)}/${encodeURIComponent(secretID)}`, { method: "DELETE", credentials: "same-origin" })
}
