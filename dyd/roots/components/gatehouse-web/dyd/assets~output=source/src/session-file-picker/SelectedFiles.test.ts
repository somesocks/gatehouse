import { describe, expect, it } from "vitest"
import { render } from "svelte/server"
import SelectedFiles from "./SelectedFiles.svelte"

describe("selected file chips", () => {
  it("uses chat attachment styling with per-file remove and delegated download controls", () => {
    const props = { files: [{ id: "sfi_a", name: "a.txt", size: 4 }, { id: "sfi_b", name: "b.txt", unavailable: true }], onRemove: () => {} }
    const body = render(SelectedFiles, { props: { ...props, onOpen: async () => {} } }).body
    expect(body.match(/attachment-chip badge/g)).toHaveLength(2)
    expect(body).toContain('aria-label="Remove a.txt"')
    expect(body).toContain('aria-label="Remove b.txt"')
    expect(body).toContain('aria-label="Download a.txt"')
    expect(body).not.toContain('aria-label="Download b.txt"')
    expect(render(SelectedFiles, { props }).body).not.toContain('aria-label="Download')
  })
})
