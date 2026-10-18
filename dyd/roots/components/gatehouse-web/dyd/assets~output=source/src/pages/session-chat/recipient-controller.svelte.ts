import { fetchChatAgents, fetchChatRecipients, type ChatAgent, type MessageTarget, type MessageTargetRef } from "../../app/chat"
import { readSavedTargets, recipientStorageKey, saveTargets, targetKey, uniqueTargets } from "./message-targets"

export function createRecipientController() {
  const state = $state({
    options: [] as MessageTarget[],
    targets: [] as MessageTargetRef[],
    ready: false,
    error: "",
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
    state.targets = []
    state.ready = false
    state.error = ""
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
      state.options = pages.flat().sort((a, b) => (a.alias ?? a.name ?? a.ref.id).localeCompare(b.alias ?? b.name ?? b.ref.id))
      if (seed !== undefined && revision === selectionRevision) {
        const id = seed ?? defaults?.find((agent) => agent.default)?.id ?? "unavailable-default"
        select([{ kind: "agent", id }])
      }
      state.ready = true
      state.error = ""
      return true
    } catch {
      if (value === generation && !combined.aborted) state.error = "Recipients could not be loaded. Try again."
      return false
    }
  }

  function available(ref: MessageTargetRef): boolean {
    return state.options.some((option) => targetKey(option.ref) === targetKey(ref))
  }

  return {
    state, start, stop, refresh, select, available,
    get canSend() { return state.ready && state.error === "" && state.targets.every(available) },
    target: (ref: MessageTargetRef): MessageTarget => state.options.find((option) => targetKey(option.ref) === targetKey(ref)) ?? { ref },
  }
}
