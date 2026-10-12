import { describe, expect, it, vi } from "vitest"
import { importClientModule } from "../testing/client-module"

describe("native collection navigation", () => {
  it("retries the failed filter rather than silently reloading the previous results", async () => {
    const { createDirectoryController } = await importClientModule(
      new URL("./directory-controller.svelte.ts", import.meta.url),
    )
    const fetchPage = vi.fn(async () => ({
      response: Response.json([]),
      items: ["original"],
      next: "original-next",
    }))
    const controller = createDirectoryController({
      fetchPage,
      onAuthenticationLost: vi.fn(),
    })
    await controller.search()
    controller.state.search = "new filter"
    fetchPage.mockResolvedValueOnce({
      response: new Response(null, { status: 503 }),
      items: [],
      next: "",
    })
    expect(await controller.search()).toBe(false)
    expect(controller.state.items).toEqual(["original"])
    fetchPage.mockResolvedValueOnce({
      response: Response.json([]),
      items: ["new result"],
      next: "",
    })
    expect(await controller.retry()).toBe(true)
    expect(fetchPage.mock.calls.at(-1)?.slice(0, 3)).toEqual([
      "new filter",
      "",
      "",
    ])
    expect(controller.state.items).toEqual(["new result"])
    expect(controller.state.history).toEqual([])
    controller.stop()
  })
  it("keeps the applied filter, continuation and history when a search fails", async () => {
    const { createCollectionPage } = await importClientModule(
      new URL("./collection-page.svelte.ts", import.meta.url),
    )
    const page = createCollectionPage()
    page.accept(new Response(null, { headers: { "X-Next-Cursor": "second" } }))
    await page.next(async () => {
      page.accept(new Response(null, { headers: { "X-Next-Cursor": "third" } }))
      return true
    })
    page.state.search = "new filter"
    let finish!: (value: boolean) => void
    const pending = page.search(async () => {
      expect(page.params()).toEqual({
        name: "new filter",
        cursor: "",
        limit: 50,
      })
      return await new Promise<boolean>((resolve) => {
        finish = resolve
      })
    })
    expect(page.state.query).toBe("")
    expect(page.state.cursor).toBe("second")
    expect(page.state.history).toEqual([""])
    finish(false)
    await pending
    expect(page.state.next).toBe("third")
    expect(page.state.history).toEqual([""])
    await page.previous(async () => true)
    expect(page.state.cursor).toBe("")
    expect(page.state.history).toEqual([])
  })

  it("retains an empty successful directory and prevents obsolete decoded responses from taking ownership", async () => {
    const { createDirectoryController } = await importClientModule(
      new URL("./directory-controller.svelte.ts", import.meta.url),
    )
    let decode!: (result: unknown) => void
    const fetchPage = vi.fn(async () => ({
      response: Response.json([]),
      items: [],
      next: "",
    }))
    const controller = createDirectoryController({
      fetchPage,
      onAuthenticationLost: vi.fn(),
    })
    await controller.reload()
    expect(controller.state.hasLoaded).toBe(true)
    fetchPage.mockImplementationOnce(
      async () =>
        await new Promise((resolve) => {
          decode = resolve
        }),
    )
    const pending = controller.reload()
    expect(controller.state.loading).toBe(false)
    expect(controller.state.reloading).toBe(true)
    fetchPage.mockResolvedValueOnce({
      response: Response.json([]),
      items: ["current"],
      next: "next",
    })
    await controller.search()
    decode({
      response: new Response(null, { status: 401 }),
      items: ["obsolete"],
    })
    await pending
    expect(controller.state.items).toEqual(["current"])
    expect(controller.state.error).toBe("")
    expect(controller.state.busy).toBe(false)
    controller.stop()
  })
})
