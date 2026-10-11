import { describe, expect, it } from "vitest"
import { createReferenceSelection } from "./reference-selection.svelte"

describe("reference selection dialog", () => {
  it("retains the committed reference and label until the choice is confirmed", () => {
    const selection = createReferenceSelection()
    selection.begin("amd_existing", "Existing model")
    selection.choose("amd_candidate", "Candidate model")
    expect(selection.state.initial).toBe("amd_existing")
    expect(selection.state.initialLabel).toBe("Existing model")
    selection.cancel()
    expect(selection.state.draft).toBe("amd_existing")
    expect(selection.state.label).toBe("Existing model")
    selection.choose("amd_candidate", "Candidate model")
    expect(selection.confirm()).toBe("amd_candidate")
    expect(selection.state.initialLabel).toBe("Candidate model")
  })

  it("keeps a draft selection when its search page changes and resets on reopening", () => {
    const selection = createReferenceSelection()
    selection.begin("", "")
    selection.choose("prn_page_two", "Selected principal")
    // Search results belong to the resource controller; replacing a page
    // must not discard the dialog's pending selection.
    expect(selection.state.draft).toBe("prn_page_two")
    selection.cancel()
    selection.begin("prn_elsewhere", "Current principal")
    expect(selection.state.draft).toBe("prn_elsewhere")
    expect(selection.state.label).toBe("Current principal")
  })
})
