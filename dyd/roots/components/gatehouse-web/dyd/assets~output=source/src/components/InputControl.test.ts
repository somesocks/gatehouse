import { createRawSnippet } from "svelte"
import { render } from "svelte/server"
import { describe, expect, it } from "vitest"
import InputControl from "./InputControl.svelte"

describe("input control focus stability", () => {
  it("keeps its trailing control mounted while showing the saving spinner", () => {
    const children = createRawSnippet(() => ({ render: () => '<input id="answer" type="text">' }))
    const trailing = createRawSnippet(() => ({ render: () => '<button type="button" aria-label="Clear Answer"></button>' }))
    const { body } = render(InputControl, {
      props: { children, trailing, loading: true, loadingLabel: "Saving Answer" },
    })

    expect(body).toContain('<input id="answer" type="text">')
    expect(body).toContain('aria-label="Clear Answer"')
    expect(body).toContain('role="status"')
    expect(body).toContain("Saving Answer")
  })
})
