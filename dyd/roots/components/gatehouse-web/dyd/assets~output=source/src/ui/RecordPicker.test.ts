import { render } from "svelte/server"
import { describe, expect, it, vi } from "vitest"
import RecordPicker from "./RecordPicker.svelte"

describe("reference field", () => {
  it("renders an input-styled dialog button with a separate clear action", () => {
    const onsearch = vi.fn()
    const { body } = render(RecordPicker, {
      props: {
        label: "Model",
        value: "amd_selected",
        selectedLabel: "Production model",
        items: [],
        onsearch,
        onprevious: vi.fn(),
        onnext: vi.fn(),
      },
    })
    expect(body).toContain('aria-haspopup="dialog"')
    expect(body).toMatch(
      /<button\b[^>]*type="button"[^>]*aria-labelledby="[^"]+"[^>]*aria-haspopup="dialog"/,
    )
    expect(body).toContain('aria-expanded="false"')
    expect(body).toContain("Production model")
    expect(body).toContain("amd_selected")
    expect(body).toContain('aria-label="Clear model selection"')
    const buttons = [...body.matchAll(/<button\b[^>]*>(.*?)<\/button>/gs)]
    expect(buttons).toHaveLength(2)
    expect(buttons[0][1]).not.toMatch(/<button\b/)
    expect(body).not.toMatch(/<input\b/)
    expect(body).not.toContain('role="combobox"')
    expect(onsearch).not.toHaveBeenCalled()
  })
  it("uses friendly workspace labels without exposing a selected ID, including before hydration", () => {
    const props = {
      label: "Project",
      value: "prj_internal",
      showIdentifiers: false,
      items: [],
      onsearch: vi.fn(),
      onprevious: vi.fn(),
      onnext: vi.fn(),
    }
    const named = render(RecordPicker, {
      props: { ...props, selectedLabel: "Roadmap" },
    }).body
    expect(named).toContain("Roadmap")
    expect(named).not.toContain("prj_internal")
    const unresolved = render(RecordPicker, { props }).body
    expect(unresolved).toContain("Selected project")
    expect(unresolved).not.toContain("prj_internal")
  })
})
