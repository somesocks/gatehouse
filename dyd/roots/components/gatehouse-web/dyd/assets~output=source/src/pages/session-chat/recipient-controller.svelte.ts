import { fetchChatAgents, fetchChatRecipients, type ChatAgent, type MessageTarget, type MessageTargetRef } from "../../app/chat"
import { readSavedTargets, recipientStorageKey, saveTargets, targetKey, uniqueTargets } from "./message-targets"

export function createRecipientController() {
  const state = $state({
    options: [] as MessageTarget[],
    inChat: [] as MessageTarget[],
    outside: [] as MessageTarget[],
    canManage: false,
    targets: [] as MessageTargetRef[],
    ready: false,
    error: "",
    outsideReady: false,
    outsideError: "",
  })
  let generation = 0
  let request: AbortController | undefined
  let context: { workspace: string; session: string; key: string; signal: AbortSignal } | undefined
  let seed: string | null | undefined
  let revision = 0

  function storage(): Storage | undefined {
    try { return window.localStorage } catch { return undefined }
  }

  function select(refs: MessageTargetRef[]): void {
    revision += 1
    seed = undefined
    state.targets = uniqueTargets(refs)
    const saved = storage()
    if (context && saved) saveTargets(saved, context.key, state.targets)
  }

  function stop(): void {
    generation += 1
    request?.abort()
    context = undefined
    state.options = []
    state.inChat = []
    state.outside = []
    state.canManage = false
    state.targets = []
    state.ready = false
    state.error = ""
    state.outsideReady = false
    state.outsideError = ""
    seed = undefined
  }

  function start(principal: string, workspace: string, session: string, signal: AbortSignal, initialAgent?: string | null): void {
    stop()
    const key = recipientStorageKey(principal, workspace, session)
    context = { workspace, session, key, signal }
    const saved = storage()
    const restored = saved ? readSavedTargets(saved, key) : null
    seed = restored === null ? initialAgent : undefined
    state.targets = restored ?? (typeof seed === "string" ? [{ kind: "agent", id: seed }] : [])
    void refresh()
  }

  async function refresh(parent?: AbortSignal): Promise<boolean> {
    if (!context) return false
    const { workspace, session, signal } = context
    const value = ++generation
    const selectionRevision = revision
    request?.abort()
    request = new AbortController()
    const combined = AbortSignal.any([signal, request.signal, ...(parent ? [parent] : [])])
    state.outsideReady = false
    state.outsideError = ""
    try {
      const kinds = ["agent", "principal", "group"] as const
      const [pages, defaults] = await Promise.all([
        Promise.all(kinds.map((kind) => fetchChatRecipients(workspace, session, kind, combined))),
        seed === null ? fetchChatAgents(workspace, combined).then(async (response) => {
          if (!response.ok) throw new Error("Recipients could not be loaded.")
          return await response.json() as ChatAgent[]
        }) : undefined,
      ])
      if (value !== generation || combined.aborted) return false
      const canManage = pages.every((page) => page.canManage)
      const inChat = pages.flatMap((page) => page.targets)
      state.inChat = inChat
      state.outside = canManage
        ? state.outside.filter((target) => !inChat.some((item) => targetKey(item.ref) === targetKey(target.ref)))
        : []
      state.canManage = canManage
      state.options = [...state.inChat, ...state.outside].sort((a, b) => (a.alias ?? a.name ?? a.ref.id).localeCompare(b.alias ?? b.name ?? b.ref.id))
      if (seed !== undefined && revision === selectionRevision) {
        const id = seed ?? defaults?.find((agent) => agent.default)?.id ?? "unavailable-default"
        select([{ kind: "agent", id }])
      }
      state.ready = true
      state.error = ""
      if (!canManage) {
        state.outsideReady = true
        return true
      }
      try {
        const outside = (await Promise.all(kinds.map((kind) => fetchChatRecipients(workspace, session, kind, combined, "out")))).flatMap((page) => page.targets)
        if (value !== generation || combined.aborted) return false
        state.outside = outside.filter((target) => !inChat.some((item) => targetKey(item.ref) === targetKey(target.ref)))
        state.options = [...state.inChat, ...state.outside].sort((a, b) => (a.alias ?? a.name ?? a.ref.id).localeCompare(b.alias ?? b.name ?? b.ref.id))
        state.outsideReady = true
      } catch {
        if (value !== generation || combined.aborted) return false
        state.outsideError = "Recipients outside this chat could not be loaded. Try again."
      }
      return true
    } catch {
      if (value === generation && !combined.aborted) state.error = "Recipients could not be loaded. Try again."
      return false
    }
  }

  function available(ref: MessageTargetRef): boolean {
    return state.options.some((option) => targetKey(option.ref) === targetKey(ref))
  }
  function inChat(ref: MessageTargetRef): boolean {
    return state.inChat.some((target) => targetKey(target.ref) === targetKey(ref))
  }

  return {
    state, start, stop, refresh, select, available,
    inChat,
    get canSend() {
      if (!context) return false
      if (state.targets.length === 0) return seed === undefined
      return state.ready && state.error === "" && state.targets.every((ref) =>
        inChat(ref) || (state.canManage && state.outsideReady && available(ref)))
    },
    target: (ref: MessageTargetRef): MessageTarget => state.options.find((option) => targetKey(option.ref) === targetKey(ref)) ?? { ref },
  }
}
