import { fetchGatehouse } from "./api"

export type ParticipantKind = "principal" | "group"
export type ParticipantRole = "member" | "contributor" | "manager"
export type SessionParticipantGrant = {
  id: string
  principal?: string
  group?: string
  role: ParticipantRole
  enabled: boolean
  revision: number
}
export type SessionParticipant = {
  id: string
  alias?: string
  name?: string
  enabled: boolean
  grants: SessionParticipantGrant[]
}
export type SessionParticipantPage = {
  participants: SessionParticipant[]
  next_cursor?: string
  can_manage_participants: boolean
}
export const participantRoles: { value: ParticipantRole; label: string }[] = [
  { value: "member", label: "Viewer" },
  { value: "contributor", label: "Participant" },
  { value: "manager", label: "Manager" },
]
export function participantName(
  item: SessionParticipant,
  kind: ParticipantKind,
) {
  return (
    item.name ??
    item.alias ??
    (kind === "principal" ? "Unnamed person" : "Unnamed group")
  )
}
function path(workspaceID: string, sessionID: string) {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/sessions/${encodeURIComponent(sessionID)}`
}
export function fetchSessionParticipants(
  workspaceID: string,
  sessionID: string,
  kind: ParticipantKind,
  name: string,
  cursor: string,
  signal: AbortSignal,
  options = false,
) {
  const query = new URLSearchParams({ kind, limit: "50" })
  if (name) query.set("name", name)
  if (cursor) query.set("cursor", cursor)
  return fetchGatehouse(
    `${path(workspaceID, sessionID)}/${options ? `${kind}-options` : "participants"}?${query}`,
    {
      credentials: "same-origin",
      signal,
    },
  )
}
export function addSessionParticipant(
  workspaceID: string,
  sessionID: string,
  kind: ParticipantKind,
  subject: string,
  role: ParticipantRole,
  signal: AbortSignal,
) {
  return fetchGatehouse(`${path(workspaceID, sessionID)}/grants`, {
    method: "POST",
    credentials: "same-origin",
    signal,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ [kind]: subject, role }),
  })
}
export function updateSessionParticipantGrant(
  workspaceID: string,
  sessionID: string,
  grant: SessionParticipantGrant,
  update: { role?: ParticipantRole; enabled?: boolean },
  signal: AbortSignal,
) {
  return fetchGatehouse(
    `${path(workspaceID, sessionID)}/grants/${encodeURIComponent(grant.id)}`,
    {
      method: "PATCH",
      credentials: "same-origin",
      signal,
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ...update, expected_revision: grant.revision }),
    },
  )
}
