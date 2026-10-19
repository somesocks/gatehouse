import { fetchChatSession } from "../app/chat"
import type { ActivityClient } from "../app/activity"
import {
  fetchSessionParticipants,
  updateSessionParticipantGrant,
  type ParticipantKind,
  type ParticipantRole,
  type SessionParticipant,
  type SessionParticipantGrant,
  type SessionParticipantPage,
} from "../app/session-participants"
import { createDirectoryController } from "../utils/directory-controller.svelte"

export type SessionParticipantsController = ReturnType<
  typeof createSessionParticipantsController
>

export function createSessionParticipantsController({
  activity,
  onAuthenticationLost,
  onAccessLost,
}: {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onAccessLost: () => void
}) {
  let workspace = ""
  let session = ""
  let generation = 0
  let operation: AbortController | undefined
  let unsubscribe: (() => void) | undefined
  const state = $state({
    open: false,
    canManage: false,
    saving: false,
    error: "",
  })
  function directory(kind: ParticipantKind) {
    return createDirectoryController<SessionParticipant>({
      onAuthenticationLost,
      async fetchPage(name, cursor, _status, signal) {
        const value = generation
        const response = await fetchSessionParticipants(
          workspace,
          session,
          kind,
          name,
          cursor,
          signal,
        )
        const body = response.ok
          ? ((await response.json()) as SessionParticipantPage)
          : undefined
        signal.throwIfAborted()
        if (value !== generation)
          throw new DOMException("Obsolete chat", "AbortError")
        if (response.status === 404) onAccessLost()
        if (body) state.canManage = body.can_manage_participants
        return { response, items: body?.participants, next: body?.next_cursor }
      },
    })
  }
  const people = directory("principal")
  const groups = directory("group")
  const bots = directory("agent")
  const directories = { principal: people, group: groups, agent: bots }
  const current = (value: number, signal: AbortSignal) =>
    value === generation && !signal.aborted

  function close() {
    state.open = false
    people.stop()
    groups.stop()
    bots.stop()
    cancelMutation()
  }
  function cancelMutation() {
    operation?.abort()
    state.saving = false
  }
  function stop() {
    generation++
    close()
    unsubscribe?.()
    unsubscribe = undefined
  }
  function start(workspaceID: string, sessionID: string) {
    stop()
    workspace = workspaceID
    session = sessionID
    people.reset()
    groups.reset()
    bots.reset()
    Object.assign(state, {
      canManage: false,
      error: "",
    })
    const value = generation
    unsubscribe = activity.subscribe(
      [
        {
          name: "chat-participants",
          topic: `${workspace}/${session}`,
          events: ["session_grant.*"],
        },
        {
          name: "chat-participant-groups",
          topic: workspace,
          events: [
            "group.*",
            "group_member.*",
            "group_agent.*",
            "workspace_agent.*",
          ],
        },
      ],
      async ({ signal }) => {
        if (!current(value, signal)) return
        if (!(await reconcile(signal)) && current(value, signal))
          throw new Error("Chat access could not be refreshed.")
      },
    )
    void activity.poll()
    return stop
  }
  async function open() {
    state.open = true
    state.error = ""
    await Promise.all(
      Object.values(directories).map((directory) => directory.retry()),
    )
  }
  async function reload() {
    return (
      await Promise.all(
        Object.values(directories).map((directory) => directory.reload()),
      )
    ).every(Boolean)
  }
  async function reconcile(signal: AbortSignal): Promise<boolean> {
    const value = generation
    try {
      const response = await fetchChatSession(workspace, session, signal)
      if (!current(value, signal)) return false
      if (response.status === 401) {
        onAuthenticationLost()
        return true
      }
      if (response.status === 404) {
        close()
        onAccessLost()
        return true
      }
      if (!response.ok) return false
      return state.open ? await reload() : true
    } catch {
      return false
    }
  }
  async function mutate(
    request: (signal: AbortSignal) => Promise<Response>,
  ): Promise<boolean> {
    if (state.saving || !state.canManage || !state.open) return false
    operation?.abort()
    const own = new AbortController()
    operation = own
    const value = generation
    state.saving = true
    state.error = ""
    try {
      const response = await request(own.signal)
      const message = response.ok ? "" : await response.text()
      if (!current(value, own.signal)) return false
      if (response.status === 401) {
        onAuthenticationLost()
        return false
      }
      if (response.status === 403) {
        state.canManage = false
        throw new Error("You no longer have permission to manage participants.")
      }
      if (!response.ok)
        throw new Error(
          message.trim() || "The participant could not be updated. Try again.",
        )
      const refreshed = await reload()
      if (!current(value, own.signal)) return false
      if (!refreshed)
        state.error =
          "The change was saved, but participants could not be refreshed. Try again."
      void activity.poll()
      return true
    } catch (error) {
      if (current(value, own.signal))
        state.error =
          error instanceof Error
            ? error.message
            : "The participant could not be updated."
      return false
    } finally {
      if (current(value, own.signal)) state.saving = false
    }
  }
  function update(grant: SessionParticipantGrant, role: ParticipantRole) {
    return mutate((signal) =>
      updateSessionParticipantGrant(
        workspace,
        session,
        grant,
        { role },
        signal,
      ),
    )
  }
  function remove(grant: SessionParticipantGrant) {
    return mutate((signal) =>
      updateSessionParticipantGrant(
        workspace,
        session,
        grant,
        { enabled: false },
        signal,
      ),
    )
  }
  return {
    state,
    people,
    groups,
    bots,
    directory: (kind: ParticipantKind) => directories[kind],
    start,
    stop,
    open,
    close,
    cancelMutation,
    reload,
    reconcile,
    update,
    remove,
  }
}
