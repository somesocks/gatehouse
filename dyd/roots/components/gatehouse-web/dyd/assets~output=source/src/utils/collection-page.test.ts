import { describe, expect, it } from "vitest"
import { createCollectionPage } from "./collection-page.svelte"

describe("workspace collection pagination", () => {
  it("keeps the submitted filter while browsing, returns to the previous cursor, and resets on a new search", async () => {
    const page = createCollectionPage()
    page.state.search = "needle"
    const requests: unknown[] = []
    const reload = async () => {
      requests.push(page.params())
      page.accept(
        new Response(null, {
          headers: { "X-Next-Cursor": page.state.cursor ? "" : "second" },
        }),
      )
      return true
    }
    await page.search(reload)
    page.state.search = "unsubmitted draft"
    await page.next(reload)
    await page.previous(reload)
    expect(requests).toEqual([
      { name: "needle", cursor: "", limit: 50 },
      { name: "needle", cursor: "second", limit: 50 },
      { name: "needle", cursor: "", limit: 50 },
    ])
    page.state.search = "replacement"
    await page.search(reload)
    expect(page.state.history).toEqual([])
    expect(page.params()).toEqual({
      name: "replacement",
      cursor: "",
      limit: 50,
    })
  })
  it("does not advance history on a failed page and ignores navigation completing after an owner reset", async () => {
    const page = createCollectionPage()
    page.state.next = "second"
    await page.next(async () => false)
    expect(page.state.cursor).toBe("")
    expect(page.state.next).toBe("second")
    expect(page.state.history).toEqual([])
    let finish!: (success: boolean) => void
    const pending = page.next(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    page.reset()
    finish(true)
    await pending
    expect(page.state.cursor).toBe("")
    expect(page.state.history).toEqual([])
    expect(page.state.busy).toBe(false)
  })
})
