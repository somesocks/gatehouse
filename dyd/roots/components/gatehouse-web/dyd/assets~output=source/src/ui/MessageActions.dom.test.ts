import { mount, tick, unmount } from "svelte"
import { afterEach, expect, it, vi } from "vitest"
import MessageActions from "./MessageActions.svelte"

let app: ReturnType<typeof mount> | undefined

afterEach(async () => {
  if (app) await unmount(app)
  app = undefined
  vi.restoreAllMocks()
  document.body.innerHTML = ""
})

function toolbarButton(label: string) {
  return document.querySelector<HTMLButtonElement>(
    `[data-slot="message-toolbar"] button[aria-label="${label}"]`,
  )!
}
async function openMenu() {
  await tick()
  const trigger = document.querySelector<HTMLButtonElement>(
    'button[aria-label="Message actions"]',
  )!
  for (const type of ["pointerdown", "pointerup", "click"])
    trigger.dispatchEvent(
      new PointerEvent(type, {
        bubbles: true,
        button: 0,
        detail: type === "click" ? 1 : 0,
        pointerType: "touch",
        isPrimary: true,
      }),
    )
  await vi.waitFor(() =>
    expect(document.querySelector('[role="menu"]')).not.toBeNull(),
  )
}
function menuItem(label: string) {
  return [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')].find(
    (item) => item.textContent?.trim() === label,
  )!
}

it("shares Copy and Reply between the keyboard-focusable desktop toolbar and touch menu", async () => {
  const text = "**Markdown**\n\n```ts\nconst value = '<raw>'\n```"
  const writeText = vi
    .spyOn(navigator.clipboard, "writeText")
    .mockResolvedValue(undefined)
  const onreply = vi.fn()
  app = mount(MessageActions, {
    target: document.body,
    props: { text, onreply },
  })
  const toolbar = document.querySelector<HTMLElement>(
    '[data-slot="message-toolbar"]',
  )!
  const menu = document.querySelector<HTMLElement>(
    '[data-slot="message-menu"]',
  )!
  expect(toolbar.className).toContain("group-focus-within/message:opacity-100")
  expect(toolbar.className).toContain("group-hover/message:opacity-100")
  expect(menu.classList.contains("hidden")).toBe(false)
  expect(menu.className).toContain("not_(any-pointer:coarse)")
  toolbarButton("Copy message").focus()
  expect(document.activeElement).toBe(toolbarButton("Copy message"))
  toolbarButton("Copy message").click()
  await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith(text))
  await tick()
  expect(document.querySelector('[role="status"]')).toBeNull()
  expect(document.body.textContent).not.toContain("Copied")
  expect(
    toolbarButton("Reply").querySelector("svg.lucide-message-square-reply"),
  ).not.toBeNull()
  toolbarButton("Reply").click()
  expect(onreply).toHaveBeenCalledTimes(1)
  await openMenu()
  expect(menuItem("Copy message")).toBeDefined()
  expect(
    menuItem("Reply").querySelector("svg.lucide-message-square-reply"),
  ).not.toBeNull()
  menuItem("Reply").click()
  await vi.waitFor(() => expect(onreply).toHaveBeenCalledTimes(2))
  await vi.waitFor(() =>
    expect(document.querySelector('[role="menu"]')).toBeNull(),
  )
  await openMenu()
  menuItem("Copy message").click()
  await vi.waitFor(() => expect(writeText).toHaveBeenCalledTimes(2))
  await vi.waitFor(() =>
    expect(document.querySelector('[role="menu"]')).toBeNull(),
  )
})

it("offers only Copy when replying is unavailable", async () => {
  vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined)
  app = mount(MessageActions, {
    target: document.body,
    props: { text: "Preview or pinned message" },
  })
  expect(toolbarButton("Reply")).toBeNull()
  await openMenu()
  expect(
    [...document.querySelectorAll('[role="menuitem"]')].map((item) =>
      item.textContent?.trim(),
    ),
  ).toEqual(["Copy message"])
})

it("reports clipboard failure and lets the same action retry", async () => {
  const writeText = vi
    .spyOn(navigator.clipboard, "writeText")
    .mockRejectedValueOnce(new Error("permission denied"))
    .mockResolvedValue(undefined)
  app = mount(MessageActions, {
    target: document.body,
    props: { text: "Message" },
  })
  toolbarButton("Copy message").click()
  await vi.waitFor(() =>
    expect(document.querySelector('[role="alert"]')?.textContent).toContain(
      "Could not copy message",
    ),
  )
  await openMenu()
  menuItem("Copy message").click()
  await vi.waitFor(() => expect(writeText).toHaveBeenCalledTimes(2))
  await tick()
  expect(document.querySelector('[role="alert"]')).toBeNull()
  expect(document.querySelector('[role="status"]')).toBeNull()
})
