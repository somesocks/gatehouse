import { createRawSnippet } from "svelte"
import { render } from "svelte/server"
import { expect, it } from "vitest"
import Avatar from "./Avatar.svelte"
import Message from "./Message.svelte"
import MessageReplies from "./MessageReplies.svelte"

const snippet = (html: string) =>
  createRawSnippet(() => ({ render: () => html }))

it("renders unboxed full-width messages with avatar, name, timestamp, then content", () => {
  const timestamp = "2026-01-01T13:04:00Z"
  const { body } = render(Message, {
    props: {
      avatar: snippet("<span>Person avatar</span>"),
      header: snippet("Alex"),
      timestamp,
      metadata: snippet("To: Team"),
      children: snippet("Message content"),
    },
  })
  const article = body.match(/<article\b[^>]*>/)![0]
  expect(article).toContain("w-full")
  expect(article).not.toMatch(/self-end|w-fit|max-w-|border|rounded|bg-/)
  expect(body).not.toContain('data-slot="card"')
  expect(body.match(/<header\b[^>]*>[\s\S]*?<\/header>/)![0]).not.toContain(
    "<button",
  )
  const avatar = body.match(/<div\b[^>]*data-slot="message-avatar"[^>]*>/)![0]
  expect(avatar).toContain("pt-1")
  const layout = body.match(/<div\b[^>]*data-slot="message-layout"[^>]*>/)![0]
  const content = body.match(/<div\b[^>]*data-slot="message-content"[^>]*>/)![0]
  expect(layout).toContain("gap-0")
  expect(content).toContain("gap-1")
  expect(body.indexOf("Person avatar")).toBeLessThan(body.indexOf("Alex"))
  expect(body.indexOf("Alex")).toBeLessThan(body.indexOf("<time"))
  expect(body.indexOf("</time>")).toBeLessThan(body.indexOf("To: Team"))
  expect(body.indexOf("To: Team")).toBeLessThan(body.indexOf("</header>"))
  expect(body.indexOf("</header>")).toBeLessThan(
    body.indexOf("Message content"),
  )
  const metadata = body.match(
    /<span\b[^>]*data-slot="message-metadata"[^>]*>/,
  )![0]
  expect(metadata).toContain("text-xs text-muted-foreground")
  expect(body).toContain(`datetime="${timestamp}"`)
  expect(body).toContain(`title="${new Date(timestamp).toLocaleString()}"`)
  expect(body).toContain(
    new Date(timestamp).toLocaleTimeString(undefined, {
      hour: "numeric",
      minute: "2-digit",
    }),
  )
})

it("anchors actions immediately after the header text without adding control height", () => {
  const { body } = render(Message, {
    props: {
      header: snippet("Alex"),
      timestamp: "2026-01-01T13:04:00Z",
      metadata: snippet("To: Team"),
      actions: snippet("<button>Actions</button>"),
      children: snippet("Text"),
    },
  })
  const header = body.match(/<header\b[^>]*>[\s\S]*?<\/header>/)![0]
  const actions = header.match(
    /<div\b[^>]*data-slot="message-actions"[^>]*>/,
  )![0]
  expect(header.indexOf("To: Team")).toBeLessThan(
    header.indexOf('data-slot="message-actions"'),
  )
  expect(actions).toContain("inline-grid h-0 align-middle")
  expect(actions).not.toMatch(/absolute|end-0|ms-auto/)
  expect(header).not.toContain("flex-1")
  expect(header).toContain("<button>Actions</button>")
})

it("gives reply previews their own responsive horizontal indentation", () => {
  const { body } = render(MessageReplies, {
    props: { children: snippet("Reply preview") },
  })
  expect(body).toContain('data-slot="message-replies"')
  expect(body).toContain("ms-6")
  expect(body).toContain("sm:ms-11")
  expect(body).not.toMatch(/border|rounded|bg-/)
})

it.each([
  ["person", "lucide-user-round"],
  ["agent", "lucide-bot"],
  ["group", "lucide-users-round"],
] as const)("uses the %s icon when there is no avatar image", (kind, icon) => {
  const { body } = render(Avatar, { props: { kind, label: "Display name" } })
  expect(body).toContain(`data-kind="${kind}"`)
  expect(body).toContain('aria-label="Display name"')
  expect(body).toContain(icon)
  expect(body).not.toContain("<img")
})
