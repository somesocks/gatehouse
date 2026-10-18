import { describe, expect, it } from "vitest"
import { messageTargets, readSavedTargets, recipientStorageKey, saveTargets, targetLabel } from "./message-targets"
import type { MessageTargetRef } from "../../app/chat"

describe("message recipient history and persistence", () => {
  it("keeps send-time labels even if the current bot has been renamed", () => {
    const targets = [{ ref: { kind: "agent" as const, id: "wag_one" }, alias: "original" }]
    const agents = [{ id: "wag_one", alias: "renamed", default: true }]
    expect(targetLabel(messageTargets({ targets }, agents)[0])).toBe("@original")
    expect(targetLabel(messageTargets({ agents: ["wag_one"] }, agents)[0])).toBe("@renamed")
    expect(targetLabel(messageTargets({ agents: ["wag_removed"] }, agents)[0])).toBe("@wag_removed")
  })

  it("isolates saved recipients by user and conversation and preserves an explicit clear", () => {
    const values = new Map<string, string>()
    const storage = { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value) } }
    const key = recipientStorageKey("alice", "workspace", "chat")
    const targets: MessageTargetRef[] = [{ kind: "agent", id: "wag_one" }, { kind: "group", id: "grp_team" }]
    saveTargets(storage, key, [...targets, targets[0]])
    expect(readSavedTargets(storage, key)).toEqual(targets)
    for (const other of [recipientStorageKey("bob", "workspace", "chat"), recipientStorageKey("alice", "other", "chat"), recipientStorageKey("alice", "workspace", "other")])
      expect(readSavedTargets(storage, other)).toBeNull()
    saveTargets(storage, key, [])
    expect(readSavedTargets(storage, key)).toEqual([])
  })

  it("ignores malformed or unsupported persisted data and tolerates denied storage", () => {
    for (const value of ["broken", "null", '{"version":2,"targets":[]}', '{"version":1,"targets":[{"kind":"unknown","id":"one"}]}'])
      expect(readSavedTargets({ getItem: () => value }, "key")).toBeNull()
    const denied = { getItem: () => { throw new Error("denied") }, setItem: () => { throw new Error("denied") } }
    expect(readSavedTargets(denied, "key")).toBeNull()
    expect(() => saveTargets(denied, "key", [])).not.toThrow()
  })
})
