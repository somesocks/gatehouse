import { mount, tick, unmount } from "svelte"
import { afterEach, describe, expect, it, vi } from "vitest"
import type { Route } from "../route"
import ResourceDialogFixture from "./testing/ResourceDialogFixture.svelte"
import { resources, collectionPath, readerPath, resourceRoute, createResourceAPI } from "./testing/resource-dialog-fixtures"
import { button, dialogs, heading, escape, outsideClick, editField } from "./testing/dialog-dom"

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

function follow(path: string) {
  const link = document.querySelector<HTMLAnchorElement>(`a[href="${path}"]`)
  expect(link, `Destination ${path}`).not.toBeNull()
  link!.click()
}

async function cancel(action: string) {
  if (action === "Escape") escape()
  else if (action === "outside click") await outsideClick()
  else {
    const link = [...document.querySelectorAll("a")].find((element) => element.textContent?.trim() === "Cancel")
    expect(link).toBeDefined()
    link!.click()
  }
}

describe.each(resources)("$noun resource dialog ownership", (resource) => {
  it.each(["Cancel", "Escape", "outside click"])("preserves search, pagination, and scroll through view/edit/new with %s", async (action) => {
    const navigate = vi.fn()
    const api = createResourceAPI(resource)
    vi.stubGlobal("fetch", api.request)
    app = mount(ResourceDialogFixture, { target: document.body, props: { onNavigate: navigate, initialRoute: resourceRoute(collectionPath(resource)) } })
    await vi.waitFor(() => expect(document.querySelector(`a[href="${readerPath(resource)}"]`)).not.toBeNull())
    const dialog = dialogs()[0]!
    const scroller = dialog.querySelector<HTMLElement>('[data-slot="resource-dialog-body"]')!
    const search = dialog.querySelector<HTMLInputElement>('input[type="search"]')!
    search.value = "Named"
    search.dispatchEvent(new Event("input", { bubbles: true }))
    search.form!.requestSubmit()
    await vi.waitFor(() => expect(api.request.mock.calls.some(([url]) => new URL(url, "http://localhost").searchParams.get("name") === "Named")).toBe(true))
    await vi.waitFor(() => expect(button("Next").disabled).toBe(false))
    button("Next").click()
    await vi.waitFor(() => expect(button("Previous").disabled).toBe(false))
    scroller.scrollTop = 80

    follow(readerPath(resource))
    await vi.waitFor(() => expect(heading()).toBe(resource.name))
    expect(dialogs()[0]).toBe(dialog)
    follow(`${readerPath(resource)}/edit`)
    await vi.waitFor(() => expect(document.getElementById(resource.field)).not.toBeNull())
    editField(resource.field, "Unsaved local edit")
    await tick()
    await cancel(action)
    await vi.waitFor(() => expect(heading()).toBe(resource.name))
    expect(dialog.textContent).toContain(resource.content)
    expect(api.request.mock.calls.some(([, options]) => options?.method === "PATCH")).toBe(false)

    button("Close").click()
    await vi.waitFor(() => expect(heading()).toBe(`Search ${resource.plural}`))
    expect(dialog.querySelector<HTMLInputElement>('input[type="search"]')?.value).toBe("Named")
    expect(button("Previous").disabled).toBe(false)
    await vi.waitFor(() => expect(scroller.scrollTop).toBe(80))

    follow(`${collectionPath(resource)}/new`)
    await vi.waitFor(() => expect(document.getElementById(resource.field)).not.toBeNull())
    editField(resource.field, "Unsaved creation")
    await tick()
    await cancel(action)
    await vi.waitFor(() => expect(heading()).toBe(`Search ${resource.plural}`))
    expect(dialogs()[0]).toBe(dialog)
    expect(dialog.querySelector<HTMLInputElement>('input[type="search"]')?.value).toBe("Named")
    expect(button("Previous").disabled).toBe(false)
    expect(api.request.mock.calls.some(([, options]) => options?.method === "POST")).toBe(false)
    button("Close").click()
    await vi.waitFor(() => expect(dialogs()).toHaveLength(0))
    expect(navigate).toHaveBeenLastCalledWith("/app/wsp/workspace/ses/chat")
  })

  it.each(["edit", "create"])("submits the %s form through its explicit dialog footer", async (mode) => {
    const navigate = vi.fn()
    const api = createResourceAPI(resource)
    vi.stubGlobal("fetch", api.request)
    app = mount(ResourceDialogFixture, {
      target: document.body,
      props: { onNavigate: navigate, initialRoute: resourceRoute(mode === "edit" ? `${readerPath(resource)}/edit` : `${collectionPath(resource)}/new`) },
    })
    await vi.waitFor(() => expect(document.getElementById(resource.field)).not.toBeNull())
    editField(resource.field, "Saved name")
    if (resource.noun === "secret" && mode === "create") editField("session-secret-value", "write-only-value")
    await tick()
    const save = button(`Save ${resource.noun}`)
    expect(save.closest("form")).toBeNull()
    expect(save.form?.id).toBe(resource.form)
    expect(save.disabled).toBe(false)
    save.click()
    await vi.waitFor(() => expect(heading()).toBe("Saved name"))
    expect(dialogs()).toHaveLength(1)
    const writes = api.request.mock.calls.filter(([, options]) => options?.method === (mode === "edit" ? "PATCH" : "POST"))
    expect(writes).toHaveLength(1)
    if (resource.noun === "secret" && mode === "edit") expect(JSON.parse(String(writes[0]![1]?.body))).not.toHaveProperty("value")
    expect(document.body.textContent).not.toContain("write-only-value")
    expect(navigate).toHaveBeenLastCalledWith(mode === "edit" ? readerPath(resource) : `${collectionPath(resource)}/created-${resource.noun}`)
  })

  it("blocks editor dismissal during a save and discards its late result after route navigation", async () => {
    const navigate = vi.fn()
    let routeFromHistory!: (route: Route) => void
    let finish!: (response: Response) => void
    const api = createResourceAPI(resource, undefined, () => new Promise<Response>((resolve) => { finish = resolve }))
    vi.stubGlobal("fetch", api.request)
    app = mount(ResourceDialogFixture, { target: document.body, props: {
      onNavigate: navigate,
      initialRoute: resourceRoute(`${readerPath(resource)}/edit`),
      onReady: (change) => { routeFromHistory = change },
    } })
    await vi.waitFor(() => expect(document.getElementById(resource.field)).not.toBeNull())
    editField(resource.field, "Pending save")
    await tick()
    button(`Save ${resource.noun}`).click()
    await vi.waitFor(() => expect(document.querySelector('a[aria-disabled="true"]')).not.toBeNull())
    escape()
    await outsideClick()
    await new Promise((resolve) => setTimeout(resolve, 30))
    expect(heading()).toBe(`Edit ${resource.noun}`)
    expect(navigate).not.toHaveBeenCalled()

    routeFromHistory(resourceRoute(collectionPath(resource)))
    await vi.waitFor(() => expect(heading()).toBe(`Search ${resource.plural}`))
    const write = api.request.mock.calls.find(([, options]) => options?.method === "PATCH")!
    expect(write[1]?.signal?.aborted).toBe(true)
    finish(Response.json({ ...api.data, title: "Late result", description: "Late result" }))
    await tick()
    await new Promise((resolve) => setTimeout(resolve, 10))
    expect(navigate).not.toHaveBeenCalled()
    expect(heading()).toBe(`Search ${resource.plural}`)
  })

  it("restores deep-linked edit parents and handles route back/forward without replacing the dialog shell", async () => {
    const navigate = vi.fn()
    let routeFromHistory!: (route: Route) => void
    vi.stubGlobal("fetch", createResourceAPI(resource).request)
    app = mount(ResourceDialogFixture, { target: document.body, props: {
      onNavigate: navigate,
      initialRoute: resourceRoute(`${readerPath(resource)}/edit`),
      onReady: (change) => { routeFromHistory = change },
    } })
    await vi.waitFor(() => expect(document.getElementById(resource.field)).not.toBeNull())
    const dialog = dialogs()[0]
    escape()
    await vi.waitFor(() => expect(heading()).toBe(resource.name))
    expect(navigate).toHaveBeenCalledExactlyOnceWith(readerPath(resource))
    routeFromHistory(resourceRoute(collectionPath(resource)))
    await vi.waitFor(() => expect(heading()).toBe(`Search ${resource.plural}`))
    routeFromHistory(resourceRoute(readerPath(resource)))
    await vi.waitFor(() => expect(heading()).toBe(resource.name))
    expect(dialogs()[0]).toBe(dialog)
  })

  it("preserves an editor draft across background metadata and resource refreshes", async () => {
    const refreshes: (() => Promise<void>)[] = []
    vi.stubGlobal("fetch", createResourceAPI(resource).request)
    app = mount(ResourceDialogFixture, { target: document.body, props: {
      onNavigate: vi.fn(),
      initialRoute: resourceRoute(`${readerPath(resource)}/edit`),
      onSubscribe: (refresh) => { refreshes.push(refresh) },
    } })
    await vi.waitFor(() => expect(document.getElementById(resource.field)).not.toBeNull())
    editField(resource.field, "Local draft survives refresh")
    await tick()
    expect(refreshes.length).toBeGreaterThan(0)
    await Promise.all(refreshes.map((refresh) => refresh()))
    await tick()
    expect((document.getElementById(resource.field) as HTMLInputElement).value).toBe("Local draft survives refresh")
    expect(heading()).toBe(`Edit ${resource.noun}`)
  })
})

it("owns the file dialog while metadata is loading and cancels late work on dismissal", async () => {
  let finish!: (response: Response) => void
  const request = vi.fn((_input: string, _options?: RequestInit) => new Promise<Response>((resolve) => { finish = resolve }))
  const navigate = vi.fn()
  vi.stubGlobal("fetch", request)
  app = mount(ResourceDialogFixture, { target: document.body, props: {
    onNavigate: navigate,
    initialRoute: resourceRoute("/app/wsp/workspace/ses/chat/files"),
  } })
  await vi.waitFor(() => expect(heading()).toBe("Search files"))
  expect(document.querySelector<HTMLInputElement>('input[type="search"]')?.placeholder).toBe("Search by name…")
  expect(document.body.textContent).toContain("Loading chat")
  button("Close").click()
  await vi.waitFor(() => expect(dialogs()).toHaveLength(0))
  expect(request.mock.calls[0]![1]?.signal?.aborted).toBe(true)
  finish(Response.json({ id: "chat", name: "Late metadata" }))
  await tick()
  await new Promise((resolve) => setTimeout(resolve, 10))
  expect(request).toHaveBeenCalledOnce()
  expect(navigate).toHaveBeenCalledExactlyOnceWith("/app/wsp/workspace/ses/chat")
  expect(dialogs()).toHaveLength(0)
})
