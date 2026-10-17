import {
  addSessionParticipant,
  fetchSessionParticipants,
  participantName,
  type ParticipantKind,
  type ParticipantRole,
  type SessionParticipant,
  type SessionParticipantPage,
} from "../app/session-participants"
import { createDirectoryController } from "../utils/directory-controller.svelte"

export function createSessionParticipantAddController({
  onAuthenticationLost,
  onAdded,
  onPermissionsChanged,
}: {
  onAuthenticationLost: () => void
  onAdded: () => Promise<void>
  onPermissionsChanged: () => void
}) {
  let workspace = "",
    session = ""
  let kind: ParticipantKind = "principal"
  let generation = 0
  let operation: AbortController | undefined
  const state = $state({
    open: false,
    subject: "",
    selectedName: "",
    role: "contributor" as ParticipantRole,
    saving: false,
    error: "",
  })
  const options = createDirectoryController<SessionParticipant>({
    onAuthenticationLost,
    async fetchPage(name, cursor, _status, signal) {
      const response = await fetchSessionParticipants(
        workspace,
        session,
        kind,
        name,
        cursor,
        signal,
        true,
      )
      const body = response.ok
        ? ((await response.json()) as SessionParticipantPage)
        : undefined
      return { response, items: body?.participants, next: body?.next_cursor }
    },
  })
  function close() {
    generation++
    operation?.abort()
    options.stop()
    state.open = false
    state.saving = false
  }
  function open(
    workspaceID: string,
    sessionID: string,
    subjectKind: ParticipantKind,
  ) {
    close()
    workspace = workspaceID
    session = sessionID
    kind = subjectKind
    options.reset()
    Object.assign(state, {
      open: true,
      subject: "",
      selectedName: "",
      role: "contributor",
      error: "",
    })
    void options.search()
    return close
  }
  function select(id: string) {
    state.subject = id
    const item = options.state.items.find((item) => item.id === id)
    if (item) state.selectedName = participantName(item, kind)
  }
  async function save(): Promise<boolean> {
    if (!state.open || state.saving || !state.subject) return false
    operation?.abort()
    const request = new AbortController()
    operation = request
    const value = generation
    const current = () => value === generation && !request.signal.aborted
    state.saving = true
    state.error = ""
    try {
      const response = await addSessionParticipant(
        workspace,
        session,
        kind,
        state.subject,
        state.role,
        request.signal,
      )
      const message = response.ok ? "" : await response.text()
      if (!current()) return false
      if (response.status === 401) {
        onAuthenticationLost()
        return false
      }
      if (response.status === 403) onPermissionsChanged()
      if (!response.ok)
        throw new Error(
          response.status === 403
            ? "You no longer have permission to add participants."
            : message.trim() ||
              "The participant could not be added. Try again.",
        )
      await onAdded()
      return current()
    } catch (error) {
      if (current())
        state.error =
          error instanceof Error
            ? error.message
            : "The participant could not be added."
      return false
    } finally {
      if (current()) state.saving = false
    }
  }
  return { state, options, open, close, select, save }
}
