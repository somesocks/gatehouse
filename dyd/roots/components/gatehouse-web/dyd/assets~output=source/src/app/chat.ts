import { fetchGatehouse as fetch } from "./api"

export type ChatAgent = {
  id: string
  alias: string
  label?: string
  default: boolean
}

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
  payload: {
    agent?: string
    agents?: string[]
    text?: string
    name?: string
    reason?: string
    until?: string
    code?: string
    message?: string
    description?: string
    form?: unknown
    output?: string
    result?: unknown
    turn?: number
    call_id?: string
    batch?: number
    position?: number
    attachments?: ChatFile[]
  }
  ref: { id: string }
  parent?: { id: string }
  author_principal?: { ref: { id: string }; name?: string }
  author_agent?: { id: string; workspace: { id: string } }
}

export type ChatEventTree = { event: ChatEvent; children: ChatEventTree[] }

export type ChatComposerFile = {
  file: File
  id?: string
  status: "pending" | "uploading" | "failed"
  error?: string
}

function chatAPIPath(workspaceID: string, sessionID: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/sessions/${encodeURIComponent(sessionID)}`
}

export function chatFileDownloadPath(
  workspaceID: string,
  sessionID: string,
  fileID: string,
): string {
  return `${chatAPIPath(workspaceID, sessionID)}/files/${encodeURIComponent(fileID)}/download`
}

export async function fetchChatSession(
  workspaceID: string,
  sessionID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(chatAPIPath(workspaceID, sessionID), {
    credentials: "same-origin",
    ...(signal === undefined ? {} : { signal }),
  })
}

export async function updateChatSession(
  workspaceID: string,
  sessionID: string,
  input: { name: string },
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(chatAPIPath(workspaceID, sessionID), {
    method: "PATCH",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    ...(signal === undefined ? {} : { signal }),
  })
}

export async function fetchChatEvents(
  workspaceID: string,
  sessionID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${chatAPIPath(workspaceID, sessionID)}/events?view=transcript&limit=100`,
    { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) },
  )
}

export async function fetchChatFiles(
  workspaceID: string,
  sessionID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(`${chatAPIPath(workspaceID, sessionID)}/files`, {
    credentials: "same-origin",
    ...(signal === undefined ? {} : { signal }),
  })
}

export async function fetchChatAgents(
  workspaceID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/agents`,
    { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) },
  )
}

export async function sendChatMessage(
  workspaceID: string,
  sessionID: string,
  input: { text?: string; agents?: string[]; attachments?: string[] },
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(`${chatAPIPath(workspaceID, sessionID)}/messages`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    ...(signal === undefined ? {} : { signal }),
  })
}

export async function cancelChatReply(
  workspaceID: string,
  sessionID: string,
  messageID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${chatAPIPath(workspaceID, sessionID)}/messages/${encodeURIComponent(messageID)}/cancel`,
    {
      method: "POST",
      credentials: "same-origin",
      ...(signal === undefined ? {} : { signal }),
    },
  )
}

export async function respondToChatApproval(
  workspaceID: string,
  sessionID: string,
  approvalID: string,
  decision: "approved" | "rejected",
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${chatAPIPath(workspaceID, sessionID)}/approvals/${encodeURIComponent(approvalID)}`,
    {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ decision }),
      ...(signal === undefined ? {} : { signal }),
    },
  )
}

export async function fetchChatInputLaunch(
  workspaceID: string,
  sessionID: string,
  inputID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${chatAPIPath(workspaceID, sessionID)}/inputs/${encodeURIComponent(inputID)}/open?redirect=false`,
    { credentials: "same-origin", ...(signal === undefined ? {} : { signal }) },
  )
}

export async function startChatFileUpload(
  workspaceID: string,
  sessionID: string,
  file: File,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(`${chatAPIPath(workspaceID, sessionID)}/files`, {
    method: "POST",
    credentials: "same-origin",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      name: file.name,
      ...(file.type === "" ? {} : { media_type: file.type }),
    }),
    ...(signal === undefined ? {} : { signal }),
  })
}

export async function uploadChatFile(
  uploadURL: string,
  file: File,
  signal?: AbortSignal,
): Promise<Response> {
  return await globalThis.fetch(uploadURL, {
    method: "PUT",
    body: file,
    ...(file.type === "" ? {} : { headers: { "Content-Type": file.type } }),
    ...(signal === undefined ? {} : { signal }),
  })
}

export async function finishChatFileUpload(
  workspaceID: string,
  sessionID: string,
  fileID: string,
  signal?: AbortSignal,
): Promise<Response> {
  return await fetch(
    `${chatAPIPath(workspaceID, sessionID)}/files/${encodeURIComponent(fileID)}/finish`,
    {
      method: "POST",
      credentials: "same-origin",
      ...(signal === undefined ? {} : { signal }),
    },
  )
}
