import { mount, tick, unmount } from "svelte"
import { afterEach, expect, it, vi } from "vitest"
import GroupsFixture from "./testing/GroupsFixture.svelte"
import { button, dialogs, editField, escape, heading } from "../../ui/testing/dialog-dom"

vi.mock("../../ui/Navigation.svelte", async () => ({
  default: (await import("../../ui/testing/EmptyConversation.svelte")).default,
}))

const collection = "/app/wsp/workspace/grp"
const detail = `${collection}/group`
const group = { id: "group", workspace: "workspace", alias: "team", name: "Team", enabled: true, revision: 1, role: "admin" }
const member = { principal: "principal", alias: "alice", name: "Alice", enabled: true, principal_enabled: true, revision: 1, role: "admin" }
const secret = { id: "credential", group: "group", description: "Deployment token", revision: 3, author: { id: "principal" }, created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" }
let app: ReturnType<typeof mount> | undefined

afterEach(async () => {
  if (app) await unmount(app)
  app = undefined
  vi.unstubAllGlobals()
  document.body.innerHTML = ""
})

function api() {
  return vi.fn(async (path: string, options?: RequestInit): Promise<Response> => {
    const url = new URL(path, "http://localhost")
    const resource = url.pathname
    if (options?.method === "POST" || options?.method === "PATCH") {
      if (resource.endsWith("/groups") || resource.endsWith("/groups/group"))
        return Response.json({ ...group, ...JSON.parse(String(options.body)) })
      return new Response(null, { status: 204 })
    }
    if (resource.endsWith("/group-capabilities")) return Response.json({ create: true })
    if (resource.endsWith("/groups")) return Response.json([group], { headers: url.searchParams.get("cursor") ? {} : { "X-Next-Cursor": "second" } })
    if (resource.endsWith("/groups/group")) return Response.json(group)
    if (resource.endsWith("/members")) return Response.json({ members: [member] })
    if (resource.endsWith("/agents")) return Response.json({ agents: [] })
    if (resource.endsWith("/member-options")) return Response.json({ principals: [{ id: "bob", name: "Bob", enabled: true }] })
    if (resource.endsWith("/agent-options")) return Response.json({ agents: [{ id: "bot", alias: "assistant", label: "Assistant", enabled: true }] })
    if (resource.endsWith("/secrets/credential")) return Response.json(secret)
    if (resource.endsWith("/secrets")) return Response.json({ secrets: [secret], can_manage_secrets: true })
    throw new Error(`Unexpected request: ${path}`)
  })
}

function follow(path: string) {
  const link = document.querySelector<HTMLAnchorElement>(`a[href="${path}"]`)
  expect(link, path).not.toBeNull()
  link!.click()
}

it.each([
  ["create", collection, "create", "Create group"],
  ["details", detail, "details", "Save details"],
  ["member", detail, "members", "Add member"],
  ["bot", detail, "bots", "Add bot"],
])("submits the %s dialog from its fixed footer with one dismissal control", async (kind, parent, view, submitLabel) => {
  const request = api(), navigate = vi.fn()
  vi.stubGlobal("fetch", request)
  app = mount(GroupsFixture, { target: document.body, props: { initialPath: `${parent}?view=${view}`, onNavigate: navigate } })
  await vi.waitFor(() => expect(dialogs()).toHaveLength(1))
  const dialog = dialogs()[0]!
  await vi.waitFor(() => expect(dialog.querySelector("input, [role=radio]")).not.toBeNull())
  if (kind === "create" || kind === "details") {
    const alias = dialog.querySelector<HTMLInputElement>('input[id$="alias"]')!
    editField(alias.id, "updated")
  } else {
    await vi.waitFor(() => expect(dialog.querySelector('[role="radio"]')).not.toBeNull())
    dialog.querySelector<HTMLButtonElement>('[role="radio"]')!.click()
  }
  await tick()
  expect([...dialog.querySelectorAll("footer button")].map((item) => item.textContent?.trim())).toEqual(["Cancel", submitLabel])
  const submit = button(submitLabel, dialog)
  if (kind === "create" || kind === "details") {
    expect(submit.closest("form")).toBeNull()
    expect(submit.form).not.toBeNull()
  }
  expect(submit.disabled).toBe(false)
  submit.click()
  await vi.waitFor(() => expect(dialogs()).toHaveLength(0))
  expect(request.mock.calls.filter(([, options]) => options?.method === "POST" || options?.method === "PATCH")).toHaveLength(1)
  expect(navigate).toHaveBeenLastCalledWith(detail)
})

it("retains the collection search and cursor across group detail and browser navigation", async () => {
  const request = api()
  vi.stubGlobal("fetch", request)
  let history!: (path: string) => void
  app = mount(GroupsFixture, { target: document.body, props: { initialPath: collection, onNavigate: vi.fn(), onReady: (change) => { history = change } } })
  await vi.waitFor(() => expect(document.querySelector(`a[href="${detail}"]`)).not.toBeNull())
  const search = document.querySelector<HTMLInputElement>('input[type="search"]')!
  search.value = "Team"
  search.dispatchEvent(new Event("input", { bubbles: true }))
  await tick()
  const submit = new Event("submit", { bubbles: true, cancelable: true })
  search.form!.dispatchEvent(submit)
  expect(submit.defaultPrevented).toBe(true)
  await tick()
  await vi.waitFor(() => expect(button("Next").disabled).toBe(false))
  button("Next").click()
  await vi.waitFor(() => expect(button("Previous").disabled).toBe(false))
  follow(detail)
  await vi.waitFor(() => expect(document.querySelector(`a[href="${detail}?view=details"]`)).not.toBeNull())
  history(collection)
  await tick()
  expect(document.querySelector('input[type="search"]')).toBe(search)
  expect(search.value).toBe("Team")
  expect(button("Previous").disabled).toBe(false)
  button("Previous").click()
  await vi.waitFor(() => expect(button("Previous").disabled).toBe(true))
  const lastCollectionRequest = request.mock.calls.filter(([url]) => new URL(url, "http://localhost").pathname.endsWith("/groups")).at(-1)!
  expect(new URL(lastCollectionRequest[0], "http://localhost").searchParams.get("name")).toBe("Team")
})

it("hydrates a deep-linked secret editor and preserves the value on metadata-only saves", async () => {
  const request = api(), navigate = vi.fn()
  vi.stubGlobal("fetch", request)
  app = mount(GroupsFixture, { target: document.body, props: { initialPath: `${detail}?view=secret-edit&secret=credential`, onNavigate: navigate } })
  await vi.waitFor(() => expect(dialogs()[0]?.querySelector<HTMLInputElement>("input")?.value).toBe(secret.description))
  const dialog = dialogs()[0]!
  expect(dialog.textContent).toContain("write-only")
  expect(dialog.textContent).toContain("Leave blank to keep the stored value")
  const description = dialog.querySelector<HTMLInputElement>("input")!
  editField(description.id, "Renamed token")
  await tick()
  const save = button("Save secret", dialog)
  expect(save.closest("footer")).not.toBeNull()
  expect(save.form).not.toBeNull()
  save.click()
  await vi.waitFor(() => expect(dialogs()).toHaveLength(0))
  const write = request.mock.calls.find(([, options]) => options?.method === "PATCH")!
  expect(JSON.parse(String(write[1]?.body))).toEqual({ description: "Renamed token", expected_revision: 3 })
  expect(navigate).toHaveBeenLastCalledWith(detail)
})

it("keeps failed edits in the dialog without duplicating the error on the page", async () => {
  const request = api()
  const read = request.getMockImplementation()!
  request.mockImplementation(async (path, options) => options?.method === "PATCH" ? new Response("Alias is already in use.", { status: 409 }) : read(path, options))
  vi.stubGlobal("fetch", request)
  let history!: (path: string) => void
  const refreshes: (() => Promise<void>)[] = []
  app = mount(GroupsFixture, { target: document.body, props: { initialPath: `${detail}?view=details`, onNavigate: vi.fn(), onReady: (change) => { history = change }, onSubscribe: (refresh) => refreshes.push(refresh) } })
  await vi.waitFor(() => expect(document.querySelector('input[id$="alias"]')).not.toBeNull())
  const alias = dialogs()[0]!.querySelector<HTMLInputElement>('input[id$="alias"]')!
  editField(alias.id, "local-draft")
  await Promise.all(refreshes.map((refresh) => refresh()))
  await tick()
  expect(alias.value).toBe("local-draft")
  button("Save details").click()
  await vi.waitFor(() => expect(dialogs()[0]?.textContent).toContain("Alias is already in use."))
  expect([...document.querySelectorAll('[role="alert"]')].filter((item) => item.textContent === "Alias is already in use.")).toHaveLength(1)
  expect(alias.value).toBe("local-draft")
  history(detail)
  await vi.waitFor(() => expect(dialogs()).toHaveLength(0))
  history(`${detail}?view=details`)
  await vi.waitFor(() => expect(dialogs()).toHaveLength(1))
  escape()
  await vi.waitFor(() => expect(dialogs()).toHaveLength(0))
})

it("aborts a pending creation on browser Back and ignores its late result", async () => {
  const request = api(), navigate = vi.fn()
  const read = request.getMockImplementation()!
  let finish!: (response: Response) => void
  request.mockImplementation((path, options) => options?.method === "POST" ? new Promise((resolve) => { finish = resolve }) : read(path, options))
  vi.stubGlobal("fetch", request)
  let history!: (path: string) => void
  app = mount(GroupsFixture, { target: document.body, props: { initialPath: `${collection}?view=create`, onNavigate: navigate, onReady: (change) => { history = change } } })
  await vi.waitFor(() => expect(document.getElementById("create-group-alias")).not.toBeNull())
  editField("create-group-alias", "new-group")
  await tick()
  button("Create group", dialogs()[0]).click()
  await vi.waitFor(() => expect(finish).toBeTypeOf("function"))
  escape()
  await tick()
  expect(dialogs()).toHaveLength(1)
  history(collection)
  await vi.waitFor(() => expect(dialogs()).toHaveLength(0))
  const write = request.mock.calls.find(([, options]) => options?.method === "POST")!
  expect(write[1]?.signal?.aborted).toBe(true)
  finish(Response.json(group))
  await tick()
  await new Promise((resolve) => setTimeout(resolve, 10))
  expect(navigate).not.toHaveBeenCalled()
})

it("shows removal failures only in their confirmation", async () => {
  const request = api()
  const read = request.getMockImplementation()!
  request.mockImplementation((path, options) => options?.method === "PATCH" ? Promise.resolve(new Response("Promote another admin first.", { status: 409 })) : read(path, options))
  vi.stubGlobal("fetch", request)
  app = mount(GroupsFixture, { target: document.body, props: { initialPath: detail, onNavigate: vi.fn() } })
  await vi.waitFor(() => expect(document.querySelector('button[aria-label="Remove Alice"]')).not.toBeNull())
  document.querySelector<HTMLButtonElement>('button[aria-label="Remove Alice"]')!.click()
  await vi.waitFor(() => expect(heading()).toBe("Remove member?"))
  button("Remove member", dialogs()[0]).click()
  await vi.waitFor(() => expect(dialogs()[0]?.textContent).toContain("Promote another admin first."))
  expect([...document.querySelectorAll('[role="alert"]')].filter((item) => item.textContent === "Promote another admin first.")).toHaveLength(1)
})

it("uses member action icons and stages role changes in an edit dialog", async () => {
  const request = api()
  vi.stubGlobal("fetch", request)
  app = mount(GroupsFixture, { target: document.body, props: { initialPath: detail, onNavigate: vi.fn() } })
  await vi.waitFor(() => expect(document.querySelector('button[aria-label="Edit Alice"]')).not.toBeNull())
  const edit = document.querySelector<HTMLButtonElement>('button[aria-label="Edit Alice"]')!
  const remove = document.querySelector<HTMLButtonElement>('button[aria-label="Remove Alice"]')!
  expect(edit.querySelector("svg")).not.toBeNull()
  expect(remove.querySelector("svg")).not.toBeNull()
  expect(edit.textContent?.trim()).toBe("")
  expect(remove.textContent?.trim()).toBe("")
  expect(edit.closest("li")?.textContent).toContain("Admin")
  expect(edit.closest("section")?.querySelector("select")).toBeNull()
  edit.click()
  await vi.waitFor(() => expect(heading()).toBe("Edit role for Alice"))
  const select = dialogs()[0]!.querySelector<HTMLSelectElement>("select")!
  expect(select.value).toBe("admin")
  expect(button("Save role").disabled).toBe(true)
  select.value = "member"
  select.dispatchEvent(new Event("change", { bubbles: true }))
  await tick()
  expect(dialogs()[0]?.textContent).toContain("will no longer be able to manage this group")
  expect(request.mock.calls.some(([, options]) => options?.method === "PATCH")).toBe(false)
  button("Cancel").click()
  await vi.waitFor(() => expect(dialogs()).toHaveLength(0))
  expect(request.mock.calls.some(([, options]) => options?.method === "PATCH")).toBe(false)
  edit.click()
  await vi.waitFor(() => expect(heading()).toBe("Edit role for Alice"))
  const role = dialogs()[0]!.querySelector<HTMLSelectElement>("select")!
  expect(role.value).toBe("admin")
  role.value = "member"
  role.dispatchEvent(new Event("change", { bubbles: true }))
  await tick()
  const save = button("Save role")
  expect(save.closest("footer")).not.toBeNull()
  expect(save.form).not.toBeNull()
  save.click()
  await vi.waitFor(() => expect(dialogs()).toHaveLength(0))
  const writes = request.mock.calls.filter(([, options]) => options?.method === "PATCH")
  expect(writes).toHaveLength(1)
  expect(writes[0]![0]).toMatch(/\/members\/principal$/)
  expect(JSON.parse(String(writes[0]![1]?.body))).toEqual({ role: "member", expected_revision: 1 })
})

it("preserves role drafts and revisions across refreshes and shows failed saves only in the editor", async () => {
  const request = api()
  const read = request.getMockImplementation()!
  const refreshes: (() => Promise<void>)[] = []
  let revision = 1
  request.mockImplementation((path, options) => {
    if (options?.method === "PATCH") return Promise.resolve(new Response("This membership changed elsewhere.", { status: 409 }))
    if (new URL(path, "http://localhost").pathname.endsWith("/members")) return Promise.resolve(Response.json({ members: [{ ...member, revision }] }))
    return read(path, options)
  })
  vi.stubGlobal("fetch", request)
  app = mount(GroupsFixture, { target: document.body, props: { initialPath: detail, onNavigate: vi.fn(), onSubscribe: (refresh) => refreshes.push(refresh) } })
  await vi.waitFor(() => expect(document.querySelector('button[aria-label="Edit Alice"]')).not.toBeNull())
  document.querySelector<HTMLButtonElement>('button[aria-label="Edit Alice"]')!.click()
  await vi.waitFor(() => expect(heading()).toBe("Edit role for Alice"))
  const role = dialogs()[0]!.querySelector<HTMLSelectElement>("select")!
  role.value = "member"
  role.dispatchEvent(new Event("change", { bubbles: true }))
  revision = 2
  await Promise.all(refreshes.map((refresh) => refresh()))
  await tick()
  expect(role.value).toBe("member")
  button("Save role").click()
  await vi.waitFor(() => expect(dialogs()[0]?.textContent).toContain("This membership changed elsewhere."))
  const write = request.mock.calls.find(([, options]) => options?.method === "PATCH")!
  expect(JSON.parse(String(write[1]?.body))).toEqual({ role: "member", expected_revision: 1 })
  expect(role.value).toBe("member")
  expect([...document.querySelectorAll('[role="alert"]')].filter((item) => item.textContent === "This membership changed elsewhere.")).toHaveLength(1)
  button("Reload role and replace draft").click()
  await vi.waitFor(() => expect(role.value).toBe("admin"))
  expect(button("Save role").disabled).toBe(true)
})

it("opens bot removal confirmation from an icon without immediately saving", async () => {
  const request = api()
  const read = request.getMockImplementation()!
  request.mockImplementation((path, options) => new URL(path, "http://localhost").pathname.endsWith("/agents")
    ? Promise.resolve(Response.json({ agents: [{ agent: "bot", alias: "assistant", label: "Assistant", agent_enabled: true, enabled: true, revision: 1 }] }))
    : read(path, options))
  vi.stubGlobal("fetch", request)
  app = mount(GroupsFixture, { target: document.body, props: { initialPath: detail, onNavigate: vi.fn() } })
  await vi.waitFor(() => expect(document.querySelector('button[aria-label="Remove Assistant"]')).not.toBeNull())
  const remove = document.querySelector<HTMLButtonElement>('button[aria-label="Remove Assistant"]')!
  expect(remove.querySelector("svg")).not.toBeNull()
  expect(remove.textContent?.trim()).toBe("")
  remove.click()
  await vi.waitFor(() => expect(heading()).toBe("Remove bot?"))
  expect(request.mock.calls.some(([, options]) => options?.method === "PATCH")).toBe(false)
  button("Cancel").click()
  await vi.waitFor(() => expect(dialogs()).toHaveLength(0))
  expect(request.mock.calls.some(([, options]) => options?.method === "PATCH")).toBe(false)
})
