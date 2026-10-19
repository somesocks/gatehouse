import type { MessageTarget, MessageTargetRef } from "../../app/chat"

export function targetKey(ref: MessageTargetRef): string {
  return `${ref.kind}:${ref.id}`
}

export function targetLabel(target: MessageTarget): string {
  return `@${target.alias || target.name || target.ref.id}`
}

export function uniqueTargets(refs: MessageTargetRef[]): MessageTargetRef[] {
  return [...new Map(refs.map((ref) => [targetKey(ref), { ...ref }])).values()]
}

export function recipientStorageKey(principal: string, workspace: string, session: string): string {
  return `gatehouse:message-targets:${JSON.stringify([principal, workspace, session])}`
}

export function readSavedTargets(storage: Pick<Storage, "getItem">, key: string): MessageTargetRef[] | null {
  try {
    const encoded = storage.getItem(key)
    if (encoded === null) return null
    const saved = JSON.parse(encoded)
    if (saved.version !== 1 || !Array.isArray(saved.targets)) return null
    if (!saved.targets.every((ref: unknown) => {
      if (ref === null || typeof ref !== "object") return false
      const { kind, id } = ref as Record<string, unknown>
      return ["agent", "principal", "group"].includes(String(kind)) && typeof id === "string" && id !== ""
    })) return null
    // The removed direct-mode fallback could persist this synthetic bot target.
    return uniqueTargets(saved.targets).filter((ref) => ref.kind !== "agent" || ref.id !== "unavailable-default")
  } catch {
    return null
  }
}

export function saveTargets(storage: Pick<Storage, "setItem">, key: string, targets: MessageTargetRef[]): void {
  try {
    storage.setItem(key, JSON.stringify({ version: 1, targets: uniqueTargets(targets) }))
  } catch {
    // Keep the current selection usable when browser storage is unavailable.
  }
}
