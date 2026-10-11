import { render } from "svelte/server"
import { describe, expect, it, vi } from "vitest"
import RecordPicker from "./RecordPicker.svelte"

describe("reference field", () => {
  it("renders a read-only modal trigger rather than an inline search form", () => {
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
    expect(body).toContain('role="combobox"')
    expect(body).toContain("readonly")
    expect(body).toContain('value="Production model"')
    expect(body).toContain("Choose model")
    expect(body).not.toContain('type="search"')
    expect(onsearch).not.toHaveBeenCalled()
  })
})
