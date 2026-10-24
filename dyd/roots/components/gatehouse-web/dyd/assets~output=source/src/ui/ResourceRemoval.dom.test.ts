import { mount, tick, unmount } from "svelte"
import { afterEach, describe, expect, it, vi } from "vitest"
import ResourceDialogFixture from "./testing/ResourceDialogFixture.svelte"
import { resources, collectionPath, readerPath, resourceRoute, createResourceAPI } from "./testing/resource-dialog-fixtures"
import { button, dialogs, heading, escape, outsideClick, pointerOutside } from "./testing/dialog-dom"

vi.mock("../pages/session-chat/SessionChatRoute.svelte", async () => ({
  default: (await import("./testing/EmptyConversation.svelte")).default,
}))

let app: ReturnType<typeof mount> | undefined

afterEach(async () => {
  if (app) await unmount(app)
  app = undefined
  vi.unstubAllGlobals()
  document.body.innerHTML = ""
})

describe.each(resources)("$noun removal in the mounted resource dialog", (resource) => {
  it.each(["Cancel", "Escape", "outside click"])("restores the reader and focus after Remove → %s", async (action) => {
    const navigate = vi.fn()
    const api = createResourceAPI(resource)
    vi.stubGlobal("fetch", api.request)
    app = mount(ResourceDialogFixture, { target: document.body, props: { onNavigate: navigate, initialRoute: resourceRoute(readerPath(resource)) } })
    await vi.waitFor(() => expect(heading()).toBe(resource.name))

    for (let attempt = 0; attempt < 2; attempt++) {
      const reader = dialogs()[0]!
      const trigger = button("Remove", reader)
      trigger.focus()
      trigger.click()
      await vi.waitFor(() => expect(dialogs()).toHaveLength(2))
      await tick()
      const confirmation = dialogs()[1]!
      expect(heading(confirmation)).toBe(`Remove ${resource.noun}?`)
      expect(reader.textContent).toContain(resource.content)
      expect(reader.contains(trigger)).toBe(true)
      await vi.waitFor(() => expect(document.activeElement).toBe(button("Cancel", confirmation)))

      if (action === "Cancel") button("Cancel", confirmation).click()
      else if (action === "Escape") escape()
      else await outsideClick()
      await tick()

      await vi.waitFor(() => expect(dialogs()).toHaveLength(1))
      expect(heading()).toBe(resource.name)
      await vi.waitFor(() => expect(document.activeElement).toBe(trigger))
      expect(navigate).not.toHaveBeenCalled()
    }

    button("Close").click()
    await vi.waitFor(() => expect(navigate).toHaveBeenCalledExactlyOnceWith(collectionPath(resource)))
    expect(api.request.mock.calls.some(([, options]) => options?.method === "DELETE")).toBe(false)
  })

  it("blocks dismissal during deletion and keeps failures in the confirmation for cancellation", async () => {
    const navigate = vi.fn()
    let finish!: (response: Response) => void
    const api = createResourceAPI(resource, () => new Promise<Response>((resolve) => { finish = resolve }))
    vi.stubGlobal("fetch", api.request)
    app = mount(ResourceDialogFixture, { target: document.body, props: { onNavigate: navigate, initialRoute: resourceRoute(readerPath(resource)) } })
    await vi.waitFor(() => expect(heading()).toBe(resource.name))
    button("Remove").click()
    await vi.waitFor(() => expect(dialogs()).toHaveLength(2))
    const confirmation = dialogs()[1]!
    button("Remove", confirmation).click()
    await vi.waitFor(() => expect(button("Cancel", confirmation).disabled).toBe(true))
    escape()
    await outsideClick()
    await new Promise((resolve) => setTimeout(resolve, 30))
    expect(dialogs()).toHaveLength(2)
    expect(navigate).not.toHaveBeenCalled()

    finish(new Response("Unavailable", { status: 503 }))
    await vi.waitFor(() => expect(confirmation.querySelector('[role="alert"]')).not.toBeNull())
    expect(button("Cancel", confirmation).disabled).toBe(false)
    button("Cancel", confirmation).click()
    await vi.waitFor(() => expect(dialogs()).toHaveLength(1))
    expect(heading()).toBe(resource.name)
    expect(navigate).not.toHaveBeenCalled()
  })

  it("navigates to the collection only after confirmed deletion", async () => {
    const navigate = vi.fn()
    const api = createResourceAPI(resource)
    vi.stubGlobal("fetch", api.request)
    app = mount(ResourceDialogFixture, { target: document.body, props: { onNavigate: navigate, initialRoute: resourceRoute(readerPath(resource)) } })
    await vi.waitFor(() => expect(heading()).toBe(resource.name))
    button("Remove").click()
    await vi.waitFor(() => expect(dialogs()).toHaveLength(2))
    button("Remove", dialogs()[1]!).click()
    await vi.waitFor(() => expect(navigate).toHaveBeenCalledExactlyOnceWith(collectionPath(resource)))
    await vi.waitFor(() => expect(dialogs()).toHaveLength(1))
    expect(heading()).toBe(`Search ${resource.plural}`)
    expect(api.request.mock.calls.filter(([, options]) => options?.method === "DELETE")).toHaveLength(1)
  })

  it("protects the parent even during confirmation setup", async () => {
    const navigate = vi.fn()
    vi.stubGlobal("fetch", createResourceAPI(resource).request)
    app = mount(ResourceDialogFixture, { target: document.body, props: { onNavigate: navigate, initialRoute: resourceRoute(readerPath(resource)) } })
    await vi.waitFor(() => expect(heading()).toBe(resource.name))
    button("Remove").click()
    await tick()
    pointerOutside()
    await new Promise((resolve) => setTimeout(resolve, 30))
    expect(heading(dialogs()[0])).toBe(resource.name)
    expect(navigate).not.toHaveBeenCalled()
  })
})
