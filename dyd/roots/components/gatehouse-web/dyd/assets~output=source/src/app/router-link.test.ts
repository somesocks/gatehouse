import { describe, expect, it } from "vitest"
import { routerLinkPath } from "./router-link"

const currentURL = "https://gatehouse.test/app/wsp/wsp_a/prj/prj_a/pnt"
const click = (overrides: Partial<MouseEvent> = {}) => ({ button: 0, defaultPrevented: false, metaKey: false, altKey: false, ctrlKey: false, shiftKey: false, ...overrides }) as MouseEvent

describe("routerLinkPath", () => {
  it("intercepts same-origin application links", () => {
    expect(routerLinkPath(click(), { href: "/app/wsp/wsp_a/prj", origin: "https://gatehouse.test", currentURL })).toBe("/app/wsp/wsp_a/prj")
    expect(routerLinkPath(click(), { href: "note_a?view=history#revision-2", origin: "https://gatehouse.test", currentURL })).toBe("/app/wsp/wsp_a/prj/prj_a/note_a?view=history#revision-2")
  })

  it("leaves modified, targeted, downloaded, and external links to the browser", () => {
    const options = { href: "/app/wsp/wsp_a/prj", origin: "https://gatehouse.test", currentURL }
    expect(routerLinkPath(click({ metaKey: true }), options)).toBeNull()
    expect(routerLinkPath(click(), { ...options, target: "_blank" })).toBeNull()
    expect(routerLinkPath(click(), { ...options, download: true })).toBeNull()
    expect(routerLinkPath(click(), { ...options, href: "https://example.test/" })).toBeNull()
  })
})
