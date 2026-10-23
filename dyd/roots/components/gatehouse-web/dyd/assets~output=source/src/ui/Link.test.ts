import { createRawSnippet } from "svelte"
import { render } from "svelte/server"
import { describe, expect, it } from "vitest"
import Link from "./Link.svelte"
import PaginationFooter from "./PaginationFooter.svelte"

const children = createRawSnippet(() => ({ render: () => "Edit note" }))

describe("navigation links", () => {
  it.each(["link", "default", "outline", "ghost", "record"] as const)(
    "renders %s styling as a native destination",
    (variant) => {
      const { body } = render(Link, {
        props: { href: "/app/note/edit", variant, children },
      })
      expect(body).toMatch(/<a\b[^>]*href="\/app\/note\/edit"/)
      expect(body).not.toMatch(/<button\b/)
    },
  )

  it("removes the destination and keyboard stop from a disabled button link", () => {
    const { body } = render(Link, {
      props: {
        href: "/app/note/edit",
        variant: "outline",
        disabled: true,
        tabindex: 0,
        children,
      },
    })
    const anchor = body.match(/<a\b[^>]*>/)![0]
    expect(anchor).not.toMatch(/\shref=/)
    expect(anchor).toContain('role="link"')
    expect(anchor).toContain('aria-disabled="true"')
    expect(anchor).toContain('tabindex="-1"')
    expect(anchor).not.toMatch(/\sdisabled=/)
  })

  it("renders URL-backed pagination as links and retains capability fragments", () => {
    const { body } = render(PaginationFooter, {
      props: {
        previous: true,
        next: true,
        previousHref: "/session-file-picker.html?cursor=previous#capability",
        nextHref: "/session-file-picker.html?cursor=next#capability",
      },
    })
    expect(body).toContain('href="/session-file-picker.html?cursor=previous#capability"')
    expect(body).toContain('href="/session-file-picker.html?cursor=next#capability"')
    expect(body).not.toMatch(/<button\b/)
  })

  it("disables URL-backed pagination while a request is pending", () => {
    const { body } = render(PaginationFooter, {
      props: {
        previous: true,
        next: true,
        previousHref: "/files?cursor=previous",
        nextHref: "/files?cursor=next",
        loading: true,
      },
    })
    const anchors = [...body.matchAll(/<a\b[^>]*>/g)].map(([anchor]) => anchor)
    expect(anchors).toHaveLength(2)
    for (const anchor of anchors) {
      expect(anchor).not.toMatch(/\shref=/)
      expect(anchor).toContain('aria-disabled="true"')
    }
  })
})
