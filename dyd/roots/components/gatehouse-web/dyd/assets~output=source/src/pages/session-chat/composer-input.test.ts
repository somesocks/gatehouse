import { describe, expect, it } from "vitest"
import { insertedMentionStart, shouldSubmitComposerKey } from "./composer-input"

describe("mobile composer input", () => {
  it("finds an inserted mention from the actual input without a keydown event", () => {
    expect(insertedMentionStart("Please ask @", 12, "@")).toBe(11)
    expect(insertedMentionStart("@", 1, "@")).toBe(0)
    expect(insertedMentionStart("Hi @ there", 4, "@")).toBe(3)
  })

  it("waits for composition to finish and ignores ordinary edits and email addresses", () => {
    expect(insertedMentionStart("@", 1, "@", true)).toBeUndefined()
    expect(insertedMentionStart("@", 1, "@", false)).toBe(0)
    expect(insertedMentionStart("alex@", 5, "@")).toBeUndefined()
    expect(insertedMentionStart("Hi @", 4, null)).toBeUndefined()
    expect(insertedMentionStart("Hi @", 4, "pasted text @")).toBeUndefined()
    expect(insertedMentionStart("Hi x", 4, "@")).toBeUndefined()
  })

  it("leaves Return to the mobile keyboard while preserving desktop send shortcuts", () => {
    const enter = { key: "Enter", shiftKey: false, ctrlKey: false, metaKey: false, isComposing: false, keyCode: 13 }
    expect(shouldSubmitComposerKey(enter, true)).toBe(false)
    expect(shouldSubmitComposerKey(enter, false)).toBe(true)
    for (const mobile of [true, false]) {
      expect(shouldSubmitComposerKey({ ...enter, ctrlKey: true }, mobile)).toBe(true)
      expect(shouldSubmitComposerKey({ ...enter, metaKey: true }, mobile)).toBe(true)
      expect(shouldSubmitComposerKey({ ...enter, shiftKey: true }, mobile)).toBe(false)
      expect(shouldSubmitComposerKey({ ...enter, ctrlKey: true, isComposing: true }, mobile)).toBe(false)
      expect(shouldSubmitComposerKey({ ...enter, ctrlKey: true, keyCode: 229 }, mobile)).toBe(false)
    }
    expect(shouldSubmitComposerKey({ ...enter, shiftKey: true }, false)).toBe(false)
    expect(shouldSubmitComposerKey({ ...enter, isComposing: true }, false)).toBe(false)
    expect(shouldSubmitComposerKey({ ...enter, keyCode: 229 }, false)).toBe(false)
  })
})
