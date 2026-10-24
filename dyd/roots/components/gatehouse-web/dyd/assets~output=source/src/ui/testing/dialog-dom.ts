import { expect } from "vitest"

export const dialogs = () => [...document.querySelectorAll<HTMLElement>('[role="dialog"]')]

export function button(label: string, root: Document | HTMLElement = document) {
  const found = [...root.querySelectorAll("button")].find((element) => element.textContent?.trim() === label)
  expect(found, `Button ${label}; rendered: ${root.textContent}`).toBeDefined()
  return found!
}

export function heading(dialog = dialogs()[0]) {
  return dialog?.querySelector('[role="heading"]')?.textContent
}

export function pointerOutside() {
  const overlays = document.querySelectorAll('[data-slot="dialog-overlay"]')
  overlays[overlays.length - 1]!.dispatchEvent(new PointerEvent("pointerdown", {
    pointerType: "mouse", button: 0, clientX: 100, clientY: 100, bubbles: true, cancelable: true,
  }))
}

export async function outsideClick() {
  // A real pointer action occurs after the newly opened dialog has been painted.
  await new Promise(requestAnimationFrame)
  pointerOutside()
}

export function escape() {
  document.body.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }))
}

export function editField(id: string, value: string) {
  const field = document.getElementById(id) as HTMLInputElement | HTMLTextAreaElement
  expect(field, `Field ${id}`).not.toBeNull()
  field.value = value
  field.dispatchEvent(new Event("input", { bubbles: true }))
  return field
}
