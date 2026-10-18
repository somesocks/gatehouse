import { fetchGatehouse as fetch } from "./api"
import { collectionQueryPath, type CollectionQuery } from "./collection-query"
import {
  decodeTranscriptEvent,
  decodeTranscriptMessageTarget,
  type TranscriptEvent,
  type TranscriptFile,
  type TranscriptMessageTarget,
  type TranscriptMessageTargetRef,
} from "../api-model"

export type ChatAgent = {
  id: string
  alias: string
  label?: string
  default: boolean
}

export type ChatFile = TranscriptFile
export type ChatEvent = TranscriptEvent
export type MessageTarget = TranscriptMessageTarget
export type MessageTargetRef = TranscriptMessageTargetRef

export async function fetchChatRecipients(
  workspaceID: string,
  sessionID: string,
  kind: MessageTargetRef["kind"],
  signal: AbortSignal,
): Promise<MessageTarget[]> {
  const targets: MessageTarget[] = []
  let cursor = ""
  const seenCursors = new Set<string>()
  do {
    const query = new URLSearchParams({ kind, limit: "100" })
    if (cursor) query.set("cursor", cursor)
    const response = await fetch(`${chatAPIPath(workspaceID, sessionID)}/recipients?${query}`, {
      credentials: "same-origin", signal,
    })
    if (!response.ok) throw new Error("Recipients could not be loaded.")
    const page = await response.json()
    if (!Array.isArray(page.targets) || (page.next_cursor !== undefined && typeof page.next_cursor !== "string"))
      throw new Error("Invalid recipient page.")
    targets.push(...page.targets.map(decodeTranscriptMessageTarget))
    cursor = page.next_cursor ?? ""
    if (cursor && seenCursors.has(cursor)) throw new Error("Invalid recipient cursor.")
    seenCursors.add(cursor)
  } while (cursor)
  return targets
}
type ChatEventsByKind = {
  [K in ChatEvent["kind"]]: Extract<ChatEvent, { kind: K }>
}
export interface ChatEventTree<
  out K extends ChatEvent["kind"] = ChatEvent["kind"],
> {
  event: ChatEventsByKind[K]
  children: ChatEventTree[]
}

export function decodeChatEvent(input: unknown): ChatEvent {
  const event = decodeTranscriptEvent(input)
  if (event.kind === "tool.failure" && (event.payload.output === undefined) === (event.payload.output_file === undefined)) {
    throw new Error("tool failure requires exactly one of output and output_file")
  }
  return event
}

function decodeChatEventTree(input: unknown): ChatEventTree {
  if (input === null || typeof input !== "object" || Array.isArray(input))
    throw new Error("invalid chat event tree")
  const tree = input as Record<string, unknown>
  if (!Array.isArray(tree.children))
    throw new Error("invalid chat event children")
  return {
    event: decodeChatEvent(tree.event),
    children: tree.children.map(decodeChatEventTree),
  }
}

export function decodeChatEventTrees(input: unknown): ChatEventTree[] {
  if (!Array.isArray(input)) throw new Error("invalid chat event list")
  return input.map(decodeChatEventTree)
}

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
  query?: CollectionQuery,
  options?: { author?: "principal" | "agent" | "all" },
): Promise<Response> {
  const path = collectionQueryPath(`${chatAPIPath(workspaceID, sessionID)}/files`, query)
  return await fetch(
    options?.author ? `${path}${path.includes("?") ? "&" : "?"}author=${options.author}` : path,
    {
      credentials: "same-origin",
      ...(signal === undefined ? {} : { signal }),
    },
  )
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
  input: { text?: string; targets?: MessageTargetRef[]; agents?: string[]; attachments?: string[] },
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
    `${chatAPIPath(workspaceID, sessionID)}/inputs/${encodeURIComponent(inputID)}/open`,
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
