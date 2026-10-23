import { describe, expect, it, vi } from "vitest"
import { followRouterLink, routerLinkPath } from "./router-link"

const currentURL = "https://gatehouse.test/app/wsp/wsp_a/prj/prj_a/pnt"
const click = (overrides: Partial<MouseEvent> = {}) =>
  ({
    button: 0,
    defaultPrevented: false,
    metaKey: false,
    altKey: false,
    ctrlKey: false,
    shiftKey: false,
    ...overrides,
  }) as MouseEvent

describe("routerLinkPath", () => {
  it("intercepts same-origin application links", () => {
    expect(
      routerLinkPath(click(), {
        href: "/app/wsp/wsp_a/prj",
        origin: "https://gatehouse.test",
        currentURL,
      }),
    ).toBe("/app/wsp/wsp_a/prj")
    expect(
      routerLinkPath(click(), {
        href: "note_a?view=history#revision-2",
        origin: "https://gatehouse.test",
        currentURL,
      }),
    ).toBe("/app/wsp/wsp_a/prj/prj_a/note_a?view=history#revision-2")
  })

  it("leaves modified, targeted, downloaded, and external links to the browser", () => {
    const options = {
      href: "/app/wsp/wsp_a/prj",
      origin: "https://gatehouse.test",
      currentURL,
    }
    expect(routerLinkPath(click({ metaKey: true }), options)).toBeNull()
    expect(routerLinkPath(click(), { ...options, target: "_blank" })).toBeNull()
    expect(routerLinkPath(click(), { ...options, download: true })).toBeNull()
    expect(
      routerLinkPath(click(), { ...options, href: "https://example.test/" }),
    ).toBeNull()
  })
})

describe("following a router link", () => {
  const options = {
    href: "/app/wsp/wsp_a/prj/prj_a/edit",
    origin: "https://gatehouse.test",
    currentURL,
  }
  const event = (overrides: Partial<MouseEvent> = {}) => ({
    ...click(overrides),
    preventDefault: vi.fn(),
  })

  it("performs cleanup only for intercepted navigation and preserves replacement history", () => {
    const order: string[] = []
    const navigate = vi.fn(() => order.push("navigate"))
    const cleanup = vi.fn(() => order.push("cleanup"))
    const activation = event()

    followRouterLink(activation, { ...options, replace: true }, navigate, cleanup)

    expect(activation.preventDefault).toHaveBeenCalledOnce()
    expect(order).toEqual(["cleanup", "navigate"])
    expect(navigate).toHaveBeenCalledWith(options.href, true)
  })

  it.each([
    { ctrlKey: true },
    { metaKey: true },
    { shiftKey: true },
    { altKey: true },
    { button: 1 },
    { button: 2 },
    { defaultPrevented: true },
  ])("leaves browser activation %j and the current draft untouched", (overrides) => {
    const navigate = vi.fn()
    const cleanup = vi.fn()
    const activation = event(overrides)

    followRouterLink(activation, options, navigate, cleanup)

    expect(activation.preventDefault).not.toHaveBeenCalled()
    expect(navigate).not.toHaveBeenCalled()
    expect(cleanup).not.toHaveBeenCalled()
  })

  it.each([
    { target: "_blank" },
    { download: "note.md" },
    { href: "https://external.test/resource" },
  ])("leaves native destination %j and the current view untouched", (destination) => {
    const navigate = vi.fn()
    const cleanup = vi.fn()
    const activation = event()

    followRouterLink(activation, { ...options, ...destination }, navigate, cleanup)

    expect(activation.preventDefault).not.toHaveBeenCalled()
    expect(navigate).not.toHaveBeenCalled()
    expect(cleanup).not.toHaveBeenCalled()
  })

  it("blocks disabled links without cleanup or navigation", () => {
    const navigate = vi.fn()
    const cleanup = vi.fn()
    const activation = event()

    followRouterLink(activation, { ...options, disabled: true }, navigate, cleanup)

    expect(activation.preventDefault).toHaveBeenCalledOnce()
    expect(navigate).not.toHaveBeenCalled()
    expect(cleanup).not.toHaveBeenCalled()
  })
})
