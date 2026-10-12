import { describe, expect, it } from "vitest"
import { setRecordReference } from "./record-reference-drafts"

describe("record reference clearing", () => {
  it.each(["pfi_selected", "prr_selected"])(
    "omits a cleared new %s reference from a save",
    (value) => {
      const drafts = [
        { value, sensitive: false },
        { id: "prv_other", value: "Other edit", sensitive: false },
      ]
      expect(setRecordReference(drafts, 0, "")).toEqual([drafts[1]])
    },
  )
  it("marks a stored reference for deletion while keeping its original value for undo", () => {
    const drafts = [
      { id: "prv_stored", value: "pfi_selected", sensitive: false },
    ]
    expect(setRecordReference(drafts, 0, "")).toEqual([
      { ...drafts[0], removed: true },
    ])
  })
})
