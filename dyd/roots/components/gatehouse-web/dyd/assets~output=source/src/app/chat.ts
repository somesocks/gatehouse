export type ChatAgent = { id: string; label?: string }

export type ChatFile = {
  id: string
  name: string
  media_type?: string
  size: number
  fingerprint: string
}

export type ChatEvent = {
  created_at: string
  kind: string
  payload: { agent?: string; text?: string; name?: string; reason?: string; code?: string; description?: string; output?: string; attachments?: ChatFile[] }
  ref: { id: string }
  parent?: { id: string }
  author_principal?: { ref: { id: string }; name?: string }
  author_agent?: { model: { id: string } }
}

export type ChatEventTree = { event: ChatEvent; children: ChatEventTree[] }

export type ChatComposerFile = { file: File; id?: string; status: "pending" | "uploading" | "failed"; error?: string }

function chatAPIPath(workspaceID: string, sessionID: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/sessions/${encodeURIComponent(sessionID)}`
}

export function chatFileDownloadPath(workspaceID: string, sessionID: string, fileID: string): string {
  return `${chatAPIPath(workspaceID, sessionID)}/files/${encodeURIComponent(fileID)}/download`
}

export async function fetchChatEvents(workspaceID: string, sessionID: string): Promise<Response> {
  return await fetch(`${chatAPIPath(workspaceID, sessionID)}/events?limit=100`, { credentials: "same-origin" })
}

export async function fetchChatAgents(workspaceID: string): Promise<Response> {
  return await fetch(`/api/v1/workspaces/${encodeURIComponent(workspaceID)}/agents`, { credentials: "same-origin" })
}

export async function sendChatMessage(workspaceID: string, sessionID: string, input: { text?: string; agent?: string; attachments?: string[] }): Promise<Response> {
  return await fetch(`${chatAPIPath(workspaceID, sessionID)}/messages`, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) })
}

export async function cancelChatReply(workspaceID: string, sessionID: string, messageID: string): Promise<Response> {
  return await fetch(`${chatAPIPath(workspaceID, sessionID)}/messages/${encodeURIComponent(messageID)}/cancel`, { method: "POST", credentials: "same-origin" })
}

export async function respondToChatApproval(workspaceID: string, sessionID: string, approvalID: string, decision: "approved" | "rejected"): Promise<Response> {
  return await fetch(`${chatAPIPath(workspaceID, sessionID)}/approvals/${encodeURIComponent(approvalID)}`, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ decision }) })
}

export async function startChatFileUpload(workspaceID: string, sessionID: string, file: File): Promise<Response> {
  return await fetch(`${chatAPIPath(workspaceID, sessionID)}/files`, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name: file.name, ...(file.type === "" ? {} : { media_type: file.type }) }) })
}

export async function uploadChatFile(uploadURL: string, file: File): Promise<Response> {
  return await fetch(uploadURL, { method: "PUT", body: file, ...(file.type === "" ? {} : { headers: { "Content-Type": file.type } }) })
}

export async function finishChatFileUpload(workspaceID: string, sessionID: string, fileID: string): Promise<Response> {
  return await fetch(`${chatAPIPath(workspaceID, sessionID)}/files/${encodeURIComponent(fileID)}/finish`, { method: "POST", credentials: "same-origin" })
}
