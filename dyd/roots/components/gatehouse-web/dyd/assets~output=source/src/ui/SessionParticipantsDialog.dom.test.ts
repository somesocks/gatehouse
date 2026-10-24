import { mount, tick, unmount } from "svelte"
import { afterEach, expect, it, vi } from "vitest"
import ParticipantsDialogFixture from "./testing/ParticipantsDialogFixture.svelte"
import { button, dialogs } from "./testing/dialog-dom"

let app: ReturnType<typeof mount> | undefined
afterEach(async () => {
  if (app) await unmount(app)
  app = undefined
  vi.unstubAllGlobals()
  document.body.innerHTML = ""
})

it("puts search below the tabs and preserves each participant type's search and results", async () => {
  const request = vi.fn(async (input: string) => {
    const url = new URL(input, "http://localhost")
    const kind = url.searchParams.get("kind")!
    const name = url.searchParams.get("name") ?? ""
    return Response.json({
      can_manage_participants: true,
      participants: [{ id: kind, name: `${kind}: ${name || "all"}`, enabled: true, grants: [] }],
    })
  })
  vi.stubGlobal("fetch", request)
  app = mount(ParticipantsDialogFixture, { target: document.body })
  await vi.waitFor(() => expect(button("Open participants").disabled).toBe(false))
  button("Open participants").click()
  await vi.waitFor(() => expect(dialogs()).toHaveLength(1))
  const dialog = dialogs()[0]!
  const tabs = dialog.querySelector('[role="tablist"]')!

  for (const [label, kind, noun, query] of [
    ["People", "principal", "person", "alice"],
    ["Groups", "group", "group", "builders"],
    ["Bots", "agent", "bot", "helper"],
  ]) {
    const tab = [...tabs.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find((element) => element.textContent?.trim() === label)!
    tab.click()
    await tick()
    const panel = dialog.querySelector<HTMLElement>(`[role="tabpanel"][aria-labelledby="${tab.id}"]`)!
    expect(panel).not.toBeNull()
    expect(panel.hidden).toBe(false)
    const search = panel.querySelector<HTMLInputElement>('input[type="search"]')!
    expect(panel.querySelectorAll('input[type="search"]')).toHaveLength(1)
    expect(panel.getAttribute("aria-labelledby")).toBe(tab.id)
    expect(tabs.compareDocumentPosition(search) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(search.value).toBe("")
    await vi.waitFor(() => expect(panel.textContent).toContain(`${kind}: all`))
    search.value = query!
    search.dispatchEvent(new Event("input", { bubbles: true }))
    search.form!.requestSubmit()
    await vi.waitFor(() => expect(panel.textContent).toContain(`${kind}: ${query}`))
    expect(request.mock.calls.some(([input]) => {
      const params = new URL(input, "http://localhost").searchParams
      return params.get("kind") === kind && params.get("name") === query
    })).toBe(true)
    expect(button(`Add ${noun}`, dialog)).toBeDefined()
  }

  const peopleTab = button("People", dialog)
  peopleTab.click()
  await tick()
  const peopleSearch = dialog.querySelector<HTMLInputElement>(`[role="tabpanel"][aria-labelledby="${peopleTab.id}"] input[type="search"]`)!
  expect(peopleSearch.value).toBe("alice")
  expect(peopleSearch.closest('[role="tabpanel"]')?.textContent).toContain("principal: alice")
  const groupsTab = button("Groups", dialog)
  groupsTab.click()
  await tick()
  const groupSearch = dialog.querySelector<HTMLInputElement>(`[role="tabpanel"][aria-labelledby="${groupsTab.id}"] input[type="search"]`)!
  expect(groupSearch.value).toBe("builders")
  expect(groupSearch.closest('[role="tabpanel"]')?.textContent).toContain("group: builders")
})
