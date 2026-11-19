import { mount, tick, unmount } from "svelte"
import { afterEach, beforeEach, expect, it, vi } from "vitest"
import ConversationFixture from "./testing/ConversationFixture.svelte"
import { button, editField, escape } from "../../ui/testing/dialog-dom"
import type { MessageTarget } from "../../app/chat"

vi.mock("../../ui/Navigation.svelte", async () => ({
  default: (await import("../../ui/testing/EmptyConversation.svelte")).default,
}))
vi.mock(
  "../../session-participants/SessionViewNavigation.svelte",
  async () => ({
    default: (await import("../../ui/testing/EmptyConversation.svelte"))
      .default,
  }),
)

const root = "/app/wsp/workspace/ses/session"
const ref = (id: string) => ({
  id,
  session: { id: "session", workspace: { id: "workspace" } },
})
const agent = {
  ref: { id: "assistant", workspace: { id: "workspace" } },
  enabled: false,
  label: "Archived assistant",
}
const event = (id: string, kind: string, payload: object, parent?: string) => ({
  ref: ref(id),
  kind,
  payload,
  created_at: "2026-01-01T00:00:00Z",
  ...(parent ? { parent: ref(parent) } : {}),
})
const first = {
  ...event("first", "message.text", {
    text: "Root question",
    targets: [{ ref: { kind: "group", id: "team" }, name: "Team" }],
  }),
  author_principal: { ref: { id: "principal" }, enabled: true, name: "Alex" },
}
const answer = {
  ...event(
    "answer",
    "agent.success",
    {
      text: "Answer Markdown",
      attachments: [
        {
          id: "file",
          name: "result.txt",
          size: 42,
          fingerprint: "fingerprint",
        },
      ],
    },
    "request-one",
  ),
  author_agent: agent,
  created_at: "2026-01-01T00:01:00Z",
}
const requests = ["request-one", "request-two"].map((id) => ({
  ...event(id, "agent.request", { agent: "assistant" }, "first"),
  target_agent: agent,
}))
const input = event(
  "input",
  "input.request",
  { description: "Review the form" },
  "hidden-tool",
)
const approval = event(
  "approval",
  "approval.request",
  { description: "Publish changes" },
  "hidden-tool",
)
const entry = (item: object) => ({
  event: item,
  previews: [],
  has_more_replies: false,
})
const empty = { entries: [], agent_requests: [], interactions: [] }
let app: ReturnType<typeof mount> | undefined

const setFiles = Object.getOwnPropertyDescriptor(
  HTMLInputElement.prototype,
  "files",
)!.set!
beforeEach(() => {
  // Browsers ignore a null FileList assignment. happy-dom instead stores
  // undefined from Svelte's initial binding, then throws when reading value.
  vi.spyOn(HTMLInputElement.prototype, "files", "set").mockImplementation(
    function (files) {
      if (files != null) setFiles.call(this, files)
    },
  )
})
afterEach(async () => {
  if (app) await unmount(app)
  app = undefined
  localStorage.removeItem(
    'gatehouse:message-sender-group:["principal","workspace","session"]',
  )
  localStorage.removeItem(
    'gatehouse:message-targets:["principal","workspace","session"]',
  )
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  document.body.innerHTML = ""
})
function api(recipients: MessageTarget[] = []) {
  let approved = false
  const sent: ReturnType<typeof event>[] = []
  return vi.fn(async (path: string, options?: RequestInit) => {
    const url = new URL(path, "http://localhost")
    if (url.pathname.endsWith("/messages") && options?.method === "POST") {
      const body = JSON.parse(String(options.body))
      const message = event(
        `sent-${sent.length + 1}`,
        "message.text",
        {
          text: body.text ?? "",
          targets: body.targets.map((ref: object) => ({ ref })),
          ...(body.send_as
            ? { author_group: { id: body.send_as, name: "Engineering" } }
            : {}),
        },
        body.parent,
      )
      sent.push(message)
      return Response.json(message)
    }
    if (url.pathname.endsWith("/recipients"))
      return Response.json({
        targets: recipients.filter(
          ({ ref }) => ref.kind === url.searchParams.get("kind"),
        ),
        can_manage_participants: false,
      })
    if (url.pathname.endsWith("/groups"))
      return url.searchParams.get("cursor") === "next-group"
        ? Response.json([
            { id: "engineering", name: "Engineering", enabled: true },
          ])
        : Response.json(
            [{ id: "disabled", name: "Disabled group", enabled: false }],
            {
              headers: { "X-Next-Cursor": "next-group" },
            },
          )
    if (url.pathname.endsWith("/inputs/input/open"))
      return new Response(null, { status: 403 })
    if (url.pathname.endsWith("/approvals/approval")) {
      approved = true
      return new Response(null, { status: 204 })
    }
    if (url.pathname.endsWith("/messages/request-two/cancel"))
      return new Response(null, { status: 204 })
    if (url.pathname.endsWith("/events/branch")) {
      const parent = url.searchParams.get("parent")
      if (parent)
        return Response.json({
          ...empty,
          parent: parent === "first" ? first : answer,
          entries: [
            ...(parent === "first" ? [entry(requests[0]!)] : []),
            entry(
              event("nested", "message.text", { text: "Nested reply" }, parent),
            ),
            ...sent
              .filter((message) => message.parent?.id === parent)
              .map(entry),
          ],
          agent_requests:
            parent === "first"
              ? [
                  {
                    request: ref("request-one"),
                    state: "completed",
                    outcome: answer,
                    interactions: [],
                  },
                ]
              : [],
        })
      if (url.searchParams.has("cursor"))
        return Response.json({
          ...empty,
          entries: [
            entry(
              event("later", "message.text", { text: "Later root message" }),
            ),
          ],
        })
      return Response.json({
        entries: [{ event: first, previews: requests, has_more_replies: true }],
        next_cursor: "next-root-page",
        agent_requests: [
          {
            request: ref("request-one"),
            state: "completed",
            outcome: answer,
            interactions: [],
          },
          {
            request: ref("request-two"),
            state: "waiting",
            interactions: [ref("input"), ref("approval")],
          },
        ],
        interactions: [
          { request: input },
          {
            request: approval,
            ...(approved
              ? {
                  response: event(
                    "approved",
                    "approval.success",
                    {},
                    "approval",
                  ),
                }
              : {}),
          },
        ],
      })
    }
    if (url.pathname.endsWith("/sessions/session"))
      return Response.json({
        id: "session",
        name: "Branch chat",
        created_at: "2026-01-01T00:00:00Z",
      })
    throw new Error(`Unexpected request: ${path} ${options?.method}`)
  })
}
function backLink() {
  return document.querySelector<HTMLAnchorElement>(
    '[aria-label="Branch navigation"] a',
  )!
}
function replyButton() {
  return document.querySelector<HTMLButtonElement>(
    'button[aria-label="Send reply"]',
  )!
}
function follow(path: string) {
  const link = document.querySelector<HTMLAnchorElement>(`a[href="${path}"]`)
  expect(link, path).not.toBeNull()
  link!.click()
}

it("renders two conversational previews, actual answer links, per-agent state and scoped controls without execution rows", async () => {
  const request = api(),
    navigate = vi.fn()
  vi.stubGlobal("fetch", request)
  app = mount(ConversationFixture, {
    target: document.body,
    props: { initialPath: root, onNavigate: navigate },
  })
  await vi.waitFor(() =>
    expect(document.body.textContent).toContain("Answer Markdown"),
  )
  expect(
    document.querySelectorAll('[aria-label="Reply previews"]'),
  ).toHaveLength(1)
  const messages = [
    ...document.querySelectorAll<HTMLElement>('[data-slot="message"]'),
  ]
  expect(document.querySelector('[aria-label="Copy Markdown"]')).toBeNull()
  expect(document.querySelector('[data-slot="feed-header"]')).toBeNull()
  for (const actions of document.querySelectorAll(
    '[data-slot="message-actions"]',
  )) {
    expect(actions.parentElement?.tagName).toBe("HEADER")
    expect(actions.className).toContain("inline-grid h-0")
    expect(actions.className).not.toMatch(/absolute|end-0/)
  }
  const rootMessage = messages[0]!
  const replyMessage = document.querySelector<HTMLElement>(
    '[data-purpose="output"]',
  )!
  expect(
    rootMessage
      .querySelector('[data-slot="avatar"]')
      ?.getAttribute("data-kind"),
  ).toBe("person")
  expect(
    rootMessage.querySelector('[data-slot="message-author"]')?.textContent,
  ).toBe("Alex")
  const timestamp = rootMessage.querySelector("time")!
  expect(timestamp.dateTime).toBe(first.created_at)
  const recipients = rootMessage.querySelector(
    '[aria-label="Message recipients"]',
  )!
  expect(recipients.textContent).toBe("To: @Team")
  expect(recipients.tagName).toBe("SPAN")
  expect(timestamp.nextElementSibling?.contains(recipients)).toBe(true)
  expect(timestamp.nextElementSibling?.className).toContain(
    "text-xs text-muted-foreground",
  )
  expect(recipients.closest("header")).toBe(timestamp.closest("header"))
  expect(
    timestamp.nextElementSibling?.nextElementSibling?.getAttribute("data-slot"),
  ).toBe("message-actions")
  expect(recipients.className).not.toContain("text-sm")
  expect(
    replyMessage
      .querySelector('[data-slot="avatar"]')
      ?.getAttribute("data-kind"),
  ).toBe("agent")
  expect(
    replyMessage.querySelector('[data-slot="message-author"]')?.textContent,
  ).toBe("Archived assistant")
  expect(replyMessage.querySelector("time")?.dateTime).toBe(answer.created_at)
  expect(replyMessage.closest('[data-slot="message-replies"]')).not.toBeNull()
  for (const message of messages) {
    expect(message.className).toContain("w-full")
    expect(message.className).not.toMatch(
      /self-end|w-fit|max-w-|border|rounded|bg-/,
    )
    expect(message.querySelector('[data-slot="card"]')).toBeNull()
  }
  expect(
    document.querySelector(
      '[role="status"][aria-label="Archived assistant status"]',
    )?.textContent,
  ).toContain("Waiting for input or approval")
  expect(
    document.querySelector('a[download="result.txt"]')?.getAttribute("href"),
  ).toContain("/files/file/download")
  expect(document.body.textContent).not.toMatch(
    /Task:|Thinking|hidden-tool|Show all/,
  )
  expect(
    [...document.querySelectorAll(`a[href="${root}?parent=first"]`)].some(
      (link) => link.textContent === "See all replies",
    ),
  ).toBe(true)
  expect(document.querySelector(`a[href="${root}?parent=answer"]`)).toBeNull()
  expect(
    document.querySelector(`a[href="${root}?parent=request-two"]`),
  ).toBeNull()
  await vi.waitFor(() =>
    expect(
      request.mock.calls.some(([path]) => path.endsWith("/inputs/input/open")),
    ).toBe(true),
  )
  expect(document.body.textContent).toContain("Review the form")
  editField("message", "Keep my root draft")
  const approvalActivity = button("Approve").parentElement!
  expect(approvalActivity.className).toContain("text-sm")
  expect(approvalActivity.className).toContain("text-muted-foreground")
  button("Approve").click()
  await vi.waitFor(() =>
    expect(document.body.textContent).toContain("Action approved:"),
  )
  expect(approvalActivity.isConnected).toBe(true)
  expect(approvalActivity.textContent).toContain("Action approved:")
  expect(approvalActivity.className).toContain("text-sm")
  expect(approvalActivity.className).toContain("text-muted-foreground")
  expect(
    JSON.parse(
      String(
        request.mock.calls.find(([path]) =>
          path.endsWith("/approvals/approval"),
        )![1]?.body,
      ),
    ),
  ).toEqual({ decision: "approved" })
  button("Cancel").click()
  await vi.waitFor(() =>
    expect(
      request.mock.calls.some(([path]) =>
        path.endsWith("/messages/request-two/cancel"),
      ),
    ).toBe(true),
  )
  expect(
    (document.getElementById("message") as HTMLTextAreaElement).value,
  ).toBe("Keep my root draft")
  expect(
    request.mock.calls
      .filter(([path]) => path.includes("/events"))
      .every(([path]) => path.includes("/events/branch?")),
  ).toBe(true)
})

it("previews replies to an agent answer only when that answer is a branch entry", async () => {
  const base = api()
  const navigate = vi.fn()
  const replies = ["First follow-up", "Sibling follow-up"].map((text, i) =>
    event(`follow-up-${i}`, "message.text", { text }, "answer"),
  )
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, options?: RequestInit) => {
      const url = new URL(path, "http://localhost")
      if (!url.pathname.endsWith("/events/branch")) return base(path, options)
      const parent = url.searchParams.get("parent")
      if (parent === "answer")
        return Response.json({
          ...empty,
          parent: answer,
          entries: replies.map(entry),
        })
      return Response.json({
        ...empty,
        ...(parent === "first"
          ? { parent: first, entries: [entry(requests[0]!)] }
          : { entries: [{ ...entry(first), previews: [requests[0]] }] }),
        agent_requests: [
          {
            request: requests[0]!.ref,
            state: "completed",
            outcome: answer,
            outcome_previews: replies,
            interactions: [],
          },
        ],
      })
    }),
  )
  app = mount(ConversationFixture, {
    target: document.body,
    props: { initialPath: root, onNavigate: navigate },
  })
  const previewGroup = () =>
    document.querySelector('[aria-label="Replies to agent message"]')
  await vi.waitFor(() =>
    expect(document.body.textContent).toContain("Answer Markdown"),
  )
  expect(previewGroup()).toBeNull()
  expect(document.body.textContent).not.toContain("First follow-up")
  expect(document.body.textContent).not.toContain("Sibling follow-up")
  expect(document.querySelector(`a[href="${root}?parent=answer"]`)).toBeNull()
  document
    .querySelector<HTMLButtonElement>(
      '[data-slot="message-toolbar"] button[aria-label="Reply"]',
    )!
    .click()
  await vi.waitFor(() => expect(previewGroup()).not.toBeNull())
  expect(navigate).toHaveBeenLastCalledWith(`${root}?parent=first`)
  const preview = previewGroup()!
  expect(
    preview.parentElement?.closest('[data-slot="message-replies"]'),
  ).toBeNull()
  expect(preview.previousElementSibling?.textContent).toContain(
    "Answer Markdown",
  )
  expect(
    [...preview.querySelectorAll('[data-slot="message"]')].map(
      (item) => item.textContent,
    ),
  ).toEqual([
    expect.stringContaining("First follow-up"),
    expect.stringContaining("Sibling follow-up"),
  ])
  expect(preview.querySelectorAll('button[aria-label="Reply"]')).toHaveLength(2)
  const path = `${root}?parent=answer`
  expect(preview.querySelector(`a[href="${path}"]`)?.textContent).toBe(
    "See all replies",
  )
  expect(
    document.querySelector(`a[href="${root}?parent=request-one"]`),
  ).toBeNull()
  follow(path)
  await vi.waitFor(() => {
    expect(previewGroup()).toBeNull()
    expect(document.body.textContent).toContain("Replying to")
    expect(document.body.textContent).toContain("First follow-up")
    expect(document.body.textContent).toContain("Sibling follow-up")
  })
  expect(navigate).toHaveBeenLastCalledWith(path)
})

it("keeps input activity inside the agent message before waiting, resumed work, and the final answer", async () => {
  const base = api()
  let state: "waiting" | "working" | "completed" = "waiting"
  let refresh: (() => Promise<void>) | undefined
  const submitted = {
    ...event("submitted", "input.success", {}, "input"),
    created_at: "2026-01-01T00:00:30Z",
  }
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, options?: RequestInit) => {
      const url = new URL(path, "http://localhost")
      if (
        url.pathname.endsWith("/events/branch") &&
        url.searchParams.get("parent") === "first"
      ) {
        return Response.json({
          ...empty,
          parent: first,
          entries: [entry(requests[0]!)],
          agent_requests: [
            {
              request: ref("request-one"),
              state,
              interactions: [ref("input")],
              ...(state === "completed" ? { outcome: answer } : {}),
            },
          ],
          interactions: [
            {
              request: input,
              ...(state !== "waiting" ? { response: submitted } : {}),
            },
          ],
        })
      }
      return base(path, options)
    }),
  )
  app = mount(ConversationFixture, {
    target: document.body,
    props: {
      initialPath: `${root}?parent=first`,
      onNavigate: vi.fn(),
      onSubscribe: (callback) => {
        refresh = callback
      },
    },
  })
  const status = () =>
    document.querySelector(
      '[role="status"][aria-label="Archived assistant status"]',
    )
  await vi.waitFor(() =>
    expect(status()?.textContent).toContain("Waiting for input or approval"),
  )
  const activity = document.querySelector(
    '[aria-label="Agent input and approval activity"]',
  )!
  expect(activity.textContent).toContain("Input requested:")
  const expectBefore = (content: Element | null) => {
    expect(content).not.toBeNull()
    expect(
      activity.compareDocumentPosition(content!) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0)
    const message = content!.closest('[data-slot="message"]')!
    expect(message).toBe(activity.closest('[data-slot="message"]'))
    expect(message.contains(activity)).toBe(true)
  }
  expectBefore(status())
  await vi.waitFor(() => expect(refresh).toBeDefined())

  state = "working"
  await refresh!()
  await vi.waitFor(() => expect(status()?.textContent).toContain("Working"))
  expect(status()!.className).toContain("text-sm")
  expect(status()!.className).toContain("text-muted-foreground")
  const cancel = status()!.querySelector("button")!
  expect(cancel.textContent?.trim()).toBe("Cancel")
  expect(cancel.className).toContain("text-sm")
  expect(cancel.className).not.toContain("text-[0.8rem]")
  expect(activity.textContent).toContain("Input submitted:")
  expect(activity.textContent).not.toContain("Input requested:")
  expectBefore(status())

  state = "completed"
  await refresh!()
  await vi.waitFor(() =>
    expect(
      document.querySelector('[data-purpose="output"]')?.textContent,
    ).toContain("Answer Markdown"),
  )
  expect(
    document.querySelector('[aria-label="Agent input and approval activity"]'),
  ).toBe(activity)
  expectBefore(document.querySelector('[data-purpose="output"] .prose'))
  expect(status()).toBeNull()
})

it.each([
  [{ code: "cancelled" }, "Cancelled"],
  [{ code: "cancelled", message: "cancelled" }, "Cancelled"],
  [
    { code: "cancelled", message: "Stopped by an administrator." },
    "Cancelled: Stopped by an administrator.",
  ],
  [
    { code: "provider_error", message: "Provider unavailable" },
    "Failed: Provider unavailable",
  ],
] as const)(
  "renders and copies failure %j as %s",
  async (payload, expected) => {
    const base = api()
    const writeText = vi
      .spyOn(navigator.clipboard, "writeText")
      .mockResolvedValue(undefined)
    const failure = {
      ...event("failure", "agent.failure", payload, "request-one"),
      author_agent: agent,
    }
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, options?: RequestInit) => {
        const url = new URL(path, "http://localhost")
        if (
          url.pathname.endsWith("/events/branch") &&
          url.searchParams.get("parent") === "first"
        ) {
          return Response.json({
            ...empty,
            parent: first,
            entries: [entry(requests[0]!)],
            agent_requests: [
              {
                request: ref("request-one"),
                state: payload.code === "cancelled" ? "cancelled" : "failed",
                outcome: failure,
                interactions: [],
              },
            ],
          })
        }
        return base(path, options)
      }),
    )
    app = mount(ConversationFixture, {
      target: document.body,
      props: { initialPath: `${root}?parent=first`, onNavigate: vi.fn() },
    })
    const status = () =>
      document.querySelector(
        '[role="status"][aria-label="Archived assistant status"]',
      )
    await vi.waitFor(() => expect(status()?.textContent?.trim()).toBe(expected))
    const message = status()!.closest('[data-slot="message"]')!
    message
      .querySelector<HTMLButtonElement>(
        '[data-slot="message-toolbar"] button[aria-label="Copy message"]',
      )!
      .click()
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith(expected))
  },
)

it.each([false, true])(
  "styles empty replies without adding a placeholder to attachment-only replies (attachments: %s)",
  async (withAttachments) => {
    const base = api()
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, options?: RequestInit) => {
        const url = new URL(path, "http://localhost")
        if (
          url.pathname.endsWith("/events/branch") &&
          url.searchParams.get("parent") === "first"
        ) {
          return Response.json({
            ...empty,
            parent: first,
            entries: [entry(requests[0]!)],
            agent_requests: [
              {
                request: ref("request-one"),
                state: "completed",
                interactions: [],
                outcome: {
                  ...answer,
                  payload: {
                    text: "",
                    attachments: withAttachments
                      ? answer.payload.attachments
                      : [],
                  },
                },
              },
            ],
          })
        }
        return base(path, options)
      }),
    )
    app = mount(ConversationFixture, {
      target: document.body,
      props: { initialPath: `${root}?parent=first`, onNavigate: vi.fn() },
    })
    await vi.waitFor(() =>
      expect(document.querySelector('[data-purpose="output"]')).not.toBeNull(),
    )
    const output = document.querySelector('[data-purpose="output"]')!
    const placeholder = output.querySelector("em")
    if (withAttachments) {
      expect(placeholder).toBeNull()
      expect(output.querySelector('a[download="result.txt"]')).not.toBeNull()
    } else {
      expect(placeholder?.textContent).toBe("No reply.")
      expect(placeholder?.className).toContain("text-sm")
      expect(placeholder?.className).toContain("text-muted-foreground")
    }
  },
)

it.each([0, 1, 2, 3, 4])(
  "places reply navigation correctly for %i replies",
  async (replyCount) => {
    const base = api()
    const navigate = vi.fn()
    const replies = Array.from({ length: Math.min(replyCount, 3) }, (_, i) =>
      event(
        `preview-${i}`,
        "message.text",
        { text: `Reply preview ${i}` },
        "first",
      ),
    )
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, options?: RequestInit) => {
        if (
          path.includes("/events/branch?") &&
          path.includes("parent=preview-0")
        )
          return Response.json({ ...empty, parent: replies[0] })
        if (path.includes("/events/branch?"))
          return Response.json({
            ...empty,
            entries: [
              {
                event: first,
                previews: replies,
                has_more_replies: replyCount > 3,
              },
              entry(
                event("other-root", "message.text", {
                  text: "Another root message",
                }),
              ),
            ],
          })
        return base(path, options)
      }),
    )
    app = mount(ConversationFixture, {
      target: document.body,
      props: { initialPath: root, onNavigate: navigate },
    })
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain("Root question"),
    )
    const section = document.querySelector('[aria-label="Conversation entry"]')!
    const message = section.querySelector('[data-slot="message"]')!
    const link = section.querySelector(`a[href="${root}?parent=first"]`)
    expect(
      message.querySelector(
        '[data-slot="message-toolbar"] button[aria-label="Reply"]',
      ),
    ).not.toBeNull()
    const previews = section.querySelector('[aria-label="Reply previews"]')
    if (replyCount >= 4) {
      expect(link?.textContent).toBe("See all replies")
      expect(
        section.querySelectorAll(`a[href="${root}?parent=first"]`),
      ).toHaveLength(1)
      expect(message.contains(link)).toBe(false)
      expect(previews?.contains(link)).toBe(true)
    } else {
      expect(link).toBeNull()
    }
    if (replyCount) {
      const previewMessages = [
        ...previews!.querySelectorAll('[data-slot="message"]'),
      ]
      expect(previewMessages).toHaveLength(Math.min(replyCount, 3))
      for (const preview of previewMessages) {
        expect(preview.querySelector("a")).toBeNull()
        expect(
          preview.querySelector('button[aria-label="Reply"]'),
        ).not.toBeNull()
        expect(
          preview.querySelector('button[aria-label="Copy message"]'),
        ).not.toBeNull()
        if (link)
          expect(
            preview.compareDocumentPosition(link) &
              Node.DOCUMENT_POSITION_FOLLOWING,
          ).not.toBe(0)
      }
    } else {
      expect(link).toBeNull()
      expect(previews).toBeNull()
    }
    const otherMessage = document.querySelectorAll(
      '[aria-label="Conversation entry"]',
    )[1]!
    expect(
      otherMessage.querySelector('button[aria-label="Reply"]'),
    ).not.toBeNull()
    expect(otherMessage.querySelector("a")).toBeNull()
    expect(
      [...section.querySelectorAll("a")].some(
        (item) =>
          item.textContent === "Reply" ||
          item.textContent === "Replies" ||
          item.textContent === "More replies",
      ),
    ).toBe(false)
    if (replyCount === 1) {
      previews!
        .querySelector<HTMLButtonElement>('button[aria-label="Reply"]')!
        .click()
      await vi.waitFor(() =>
        expect(
          document.querySelector('[aria-label="Thread anchor"]')?.textContent,
        ).toContain("Reply preview 0"),
      )
      expect(navigate).toHaveBeenLastCalledWith(`${root}?parent=preview-0`)
    }
  },
)

it("copies the agent preview's Markdown and replies to its actual outcome", async () => {
  const request = api(),
    navigate = vi.fn()
  const writeText = vi
    .spyOn(navigator.clipboard, "writeText")
    .mockResolvedValue(undefined)
  vi.stubGlobal("fetch", request)
  app = mount(ConversationFixture, {
    target: document.body,
    props: { initialPath: root, onNavigate: navigate },
  })
  await vi.waitFor(() =>
    expect(document.body.textContent).toContain("Answer Markdown"),
  )
  const preview = document.querySelector('[data-purpose="output"]')!
  preview
    .querySelector<HTMLButtonElement>('button[aria-label="Copy message"]')!
    .click()
  await vi.waitFor(() =>
    expect(writeText).toHaveBeenCalledWith("Answer Markdown"),
  )
  preview
    .querySelector<HTMLButtonElement>('button[aria-label="Reply"]')!
    .click()
  await vi.waitFor(() =>
    expect(
      document.querySelector('[aria-label="Thread anchor"]')?.textContent,
    ).toContain("Answer Markdown"),
  )
  expect(navigate).toHaveBeenLastCalledWith(`${root}?parent=answer`)
  expect(
    document.querySelector(
      '[aria-label="Thread anchor"] button[aria-label="Reply"]',
    ),
  ).toBeNull()
})

it("navigates to the actual selected parent, restores the root draft on history/root navigation, and loads more on demand", async () => {
  const request = api(),
    navigate = vi.fn()
  let history!: (path: string) => void
  vi.stubGlobal("fetch", request)
  app = mount(ConversationFixture, {
    target: document.body,
    props: {
      initialPath: root,
      onNavigate: navigate,
      onReady: (change) => {
        history = change
      },
    },
  })
  await vi.waitFor(() =>
    expect(document.getElementById("message")).not.toBeNull(),
  )
  editField("message", "Root draft")
  button("Load more messages").click()
  await vi.waitFor(() =>
    expect(document.body.textContent).toContain("Later root message"),
  )
  follow(`${root}?parent=first`)
  await vi.waitFor(() =>
    expect(document.body.textContent).toContain("Nested reply"),
  )
  expect(navigate).toHaveBeenLastCalledWith(`${root}?parent=first`)
  const anchor = document.querySelector('[aria-label="Thread anchor"]')!
  const feed = document.querySelector('[data-slot="feed"]')!
  expect(anchor.textContent).toContain("Root question")
  expect(anchor.textContent).toContain("Alex")
  expect(anchor.textContent).toContain("To: @Team")
  expect(anchor.querySelector("time")?.dateTime).toBe(first.created_at)
  expect(feed.contains(anchor)).toBe(false)
  const threadHeader = anchor.closest('[data-slot="feed-header"]')!
  const navigation = threadHeader.querySelector(
    '[aria-label="Branch navigation"]',
  )!
  const heading = threadHeader.querySelector<HTMLElement>("h2")!
  expect(navigation.textContent?.trim()).toBe("Back")
  expect(
    navigation.querySelector('svg.lucide-arrow-left[aria-hidden="true"]'),
  ).not.toBeNull()
  expect(heading.textContent).toBe("Replying to")
  expect(heading.className).toContain("text-base font-semibold")
  expect(heading.className).not.toContain("text-muted-foreground")
  expect(navigation.parentElement).toBe(heading.parentElement)
  expect(heading.parentElement?.className).toContain("gap-0")
  expect(heading.className).toContain("leading-tight")
  expect(heading.parentElement?.parentElement).toBe(anchor.parentElement)
  expect(anchor.parentElement?.className).toContain("gap-4")
  expect(
    navigation.compareDocumentPosition(heading) &
      Node.DOCUMENT_POSITION_FOLLOWING,
  ).not.toBe(0)
  expect(
    heading.compareDocumentPosition(anchor) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).not.toBe(0)
  expect(anchor.querySelector("a")).toBeNull()
  expect(feed.textContent).not.toContain("Root question")
  expect(document.body.textContent).not.toContain("Chat root")
  expect(backLink().getAttribute("href")).toBe(root)
  expect(backLink().className).toContain("text-sm")
  expect(backLink().className).toContain("text-primary")
  expect(backLink().dataset.slot).not.toBe("button")
  expect(navigation.querySelector("button")).toBeNull()
  expect(
    document.querySelector(
      '[data-purpose="output"] button[aria-label="Reply"]',
    ),
  ).not.toBeNull()
  expect(
    (document.getElementById("message") as HTMLTextAreaElement).value,
  ).toBe("Root draft")
  expect(
    (document.getElementById("message") as HTMLTextAreaElement).placeholder,
  ).toBe("Write a reply")
  const selected = new URL(
    request.mock.calls
      .filter(([path]) => path.includes("/events/branch"))
      .at(-1)![0],
    "http://localhost",
  )
  expect(selected.searchParams.get("parent")).toBe("first")
  expect(selected.searchParams.has("cursor")).toBe(false)
  history(root)
  await vi.waitFor(() =>
    expect(document.getElementById("message")).not.toBeNull(),
  )
  expect(
    (document.getElementById("message") as HTMLTextAreaElement).value,
  ).toBe("Root draft")
  history(`${root}?parent=first`)
  await vi.waitFor(() =>
    expect(document.body.textContent).toContain("Nested reply"),
  )
  backLink().click()
  expect(navigate).toHaveBeenLastCalledWith(root)
  await vi.waitFor(() =>
    expect(document.getElementById("message")).not.toBeNull(),
  )
  expect(
    (document.getElementById("message") as HTMLTextAreaElement).value,
  ).toBe("Root draft")
})

it.each(["first", "answer"])(
  "sends a reply to %s from the thread composer without inheriting parent recipients or files",
  async (parentID) => {
    const request = api()
    vi.stubGlobal("fetch", request)
    app = mount(ConversationFixture, {
      target: document.body,
      props: { initialPath: `${root}?parent=${parentID}`, onNavigate: vi.fn() },
    })
    await vi.waitFor(() =>
      expect(
        document.querySelector('[aria-label="Thread anchor"]'),
      ).not.toBeNull(),
    )
    expect(document.body.textContent).not.toContain(
      "Reply composition is not available yet",
    )
    editField("message", "Thread follow-up")
    await vi.waitFor(() => expect(replyButton().disabled).toBe(false))
    replyButton().click()
    await vi.waitFor(() =>
      expect(
        document.querySelector('[data-slot="feed"]')?.textContent,
      ).toContain("Thread follow-up"),
    )
    const sends = request.mock.calls.filter(([path]) =>
      path.endsWith("/messages"),
    )
    expect(sends).toHaveLength(1)
    expect(JSON.parse(String(sends[0]![1]?.body))).toEqual({
      parent: parentID,
      text: "Thread follow-up",
      targets: [],
    })
    expect(
      (document.getElementById("message") as HTMLTextAreaElement).value,
    ).toBe("")
    expect(
      document.querySelector('[aria-label="Thread anchor"]')?.textContent,
    ).toContain(parentID === "first" ? "Root question" : "Answer Markdown")
  },
)

it("keeps the textbox above the toolbar and expands from one row to three for focus or a draft", async () => {
  vi.stubGlobal("fetch", api())
  app = mount(ConversationFixture, {
    target: document.body,
    props: { initialPath: root, onNavigate: vi.fn() },
  })
  const controls = () =>
    document.querySelector('[data-slot="composer-controls"]')!
  await vi.waitFor(() => expect(controls()).not.toBeNull())
  const expanded = () => controls().hasAttribute("data-expanded")
  const input = document.getElementById("message") as HTMLTextAreaElement
  const attach = controls().querySelector('[aria-label="Attach files"]')!
  const rows = () => Number(input.rows)
  expect(
    input.compareDocumentPosition(attach) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).not.toBe(0)
  expect(expanded()).toBe(false)
  expect(rows()).toBe(1)
  input.focus()
  await tick()
  expect(expanded()).toBe(true)
  expect(rows()).toBe(3)
  editField("message", "Draft\nwith two lines")
  input.blur()
  await tick()
  expect(expanded()).toBe(true)
  expect(rows()).toBe(3)
  editField("message", "")
  await tick()
  expect(expanded()).toBe(false)
  expect(rows()).toBe(1)
})

it("sends replies as a group until Send as is cleared, including after reopening the chat", async () => {
  const recipient = { kind: "principal", id: "alex" } as const
  const request = api([{ ref: recipient, alias: "alex" }])
  vi.stubGlobal("fetch", request)
  app = mount(ConversationFixture, {
    target: document.body,
    props: { initialPath: `${root}?parent=answer`, onNavigate: vi.fn() },
  })
  const commands = () =>
    document.querySelector<HTMLButtonElement>(
      'button[aria-label="Chat commands"]',
    )
  await vi.waitFor(() => expect(commands()).not.toBeNull())
  const trigger = commands()!
  expect(trigger.previousElementSibling?.getAttribute("aria-label")).toBe(
    "Choose recipient",
  )
  expect(trigger.type).toBe("button")
  editField("message", "Keep / in this draft")
  const picker = () => document.querySelector('[role="dialog"]')
  const item = (value: string) =>
    picker()?.querySelector<HTMLElement>(
      `[role="option"][data-value="${value}"]`,
    )
  document
    .querySelector<HTMLButtonElement>('button[aria-label="Choose recipient"]')!
    .click()
  await vi.waitFor(() =>
    expect(item("principal:alex")?.hasAttribute("data-disabled")).toBe(false),
  )
  item("principal:alex")!.click()
  await vi.waitFor(() => expect(picker()).toBeNull())
  trigger.click()
  await vi.waitFor(() => expect(item("Send as")).toBeTruthy())
  item("Send as")!.click()
  await vi.waitFor(() => expect(item("engineering")).toBeTruthy())
  expect(item("disabled")).toBeFalsy()
  item("engineering")!.click()
  const chip = () => document.querySelector('[aria-label="Message sender"]')
  await vi.waitFor(() => {
    expect(picker()).toBeNull()
    expect(chip()?.textContent).toContain("Engineering")
    expect(chip()?.parentElement?.firstElementChild?.textContent?.trim()).toBe(
      "To:",
    )
    expect(chip()?.previousElementSibling?.textContent).toContain("@alex")
  })
  const posted = () =>
    request.mock.calls
      .filter(([path]) => path.endsWith("/messages"))
      .map(([, init]) => JSON.parse(String(init?.body)))
  expect(posted()).toEqual([])
  replyButton().click()
  await vi.waitFor(() => {
    expect(posted()).toEqual([
      {
        parent: "answer",
        send_as: "engineering",
        text: "Keep / in this draft",
        targets: [recipient],
      },
    ])
    expect(document.getElementById("message")).toHaveProperty("value", "")
    expect(chip()?.textContent).toContain("Engineering")
    expect(document.body.textContent?.match(/Engineering/g)).toHaveLength(2)
  })
  await unmount(app!)
  app = mount(ConversationFixture, {
    target: document.body,
    props: { initialPath: `${root}?parent=answer`, onNavigate: vi.fn() },
  })
  await vi.waitFor(() => expect(chip()?.textContent).toContain("Engineering"))
  chip()!
    .querySelector<HTMLButtonElement>('button[aria-haspopup="dialog"]')!
    .click()
  await vi.waitFor(() => expect(item("clear")?.textContent).toContain("Clear"))
  item("clear")!.click()
  await vi.waitFor(() => {
    expect(chip()).toBeNull()
    expect(picker()).toBeNull()
  })
  editField("message", "Personal reply")
  await vi.waitFor(() => expect(replyButton().disabled).toBe(false))
  replyButton().click()
  await vi.waitFor(() =>
    expect(posted()[1]).toEqual({
      parent: "answer",
      text: "Personal reply",
      targets: [recipient],
    }),
  )
})

it("opens the composer menus for typed triggers, not pasted text", async () => {
  vi.stubGlobal("fetch", api())
  app = mount(ConversationFixture, {
    target: document.body,
    props: { initialPath: root, onNavigate: vi.fn() },
  })
  await vi.waitFor(() =>
    expect(document.getElementById("message")).not.toBeNull(),
  )
  const input = document.getElementById("message") as HTMLTextAreaElement
  function insert(
    value: string,
    data: string,
    inputType: string,
    cursor = value.length,
  ) {
    input.value = value
    input.setSelectionRange(cursor, cursor)
    input.dispatchEvent(
      new InputEvent("input", { data, inputType, bubbles: true }),
    )
  }
  input.focus()
  for (const text of ["/", "@", "https://example.com/@name"]) {
    insert(text, text, "insertFromPaste")
    await tick()
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    expect(input.value).toBe(text)
  }
  insert("A typed /", "/", "insertText")
  await vi.waitFor(() =>
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
      "Chat commands",
    ),
  )
  escape()
  await vi.waitFor(() => {
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    expect(document.activeElement).toBe(input)
  })
  expect(input.value).toBe("A typed /")
  insert("Keep a/b and /", "/", "insertText", 7)
  await vi.waitFor(() => {
    expect(
      document.querySelector(
        '[role="dialog"] [role="option"][data-value="Send as"]',
      ),
    ).toBeTruthy()
  })
  document
    .querySelector<HTMLElement>(
      '[role="dialog"] [role="option"][data-value="Send as"]',
    )!
    .click()
  await vi.waitFor(() =>
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
      "Your groups",
    ),
  )
  escape()
  await vi.waitFor(() => {
    expect(document.querySelector('[role="dialog"]')).toBeNull()
    expect(document.activeElement).toBe(input)
  })
  expect(input.value).toBe("Keep ab and /")
  expect(input.selectionStart).toBe(6)
  insert("Hello @", "@", "insertText")
  await vi.waitFor(() =>
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain(
      "Choose recipient",
    ),
  )
})

it("retains a rejected reply draft and shows the send error only once in the composer", async () => {
  const base = api()
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, options?: RequestInit) =>
      path.endsWith("/messages")
        ? new Response("invalid message parent", { status: 400 })
        : base(path, options),
    ),
  )
  app = mount(ConversationFixture, {
    target: document.body,
    props: { initialPath: `${root}?parent=answer`, onNavigate: vi.fn() },
  })
  await vi.waitFor(() =>
    expect(
      document.querySelector('[aria-label="Thread anchor"]'),
    ).not.toBeNull(),
  )
  editField("message", "Keep this reply draft")
  await vi.waitFor(() => expect(replyButton().disabled).toBe(false))
  replyButton().click()
  await vi.waitFor(() =>
    expect(document.getElementById("message-error")?.textContent).toContain(
      "invalid message parent",
    ),
  )
  expect(document.querySelectorAll('[role="alert"]')).toHaveLength(1)
  expect(
    (document.getElementById("message") as HTMLTextAreaElement).value,
  ).toBe("Keep this reply draft")
})

it("shows only the projected description for an input anchor, without opening a scoped form", async () => {
  const base = api()
  const request = vi.fn(async (path: string, options?: RequestInit) =>
    path.includes("/events/branch?")
      ? Response.json({ ...empty, parent: { ...input, author_agent: agent } })
      : base(path, options),
  )
  vi.stubGlobal("fetch", request)
  app = mount(ConversationFixture, {
    target: document.body,
    props: { initialPath: `${root}?parent=input`, onNavigate: vi.fn() },
  })
  await vi.waitFor(() =>
    expect(
      document.querySelector('[aria-label="Thread anchor"]')?.textContent,
    ).toContain("Review the form"),
  )
  expect(request.mock.calls.some(([path]) => path.includes("/inputs/"))).toBe(
    false,
  )
  expect(
    document
      .querySelector('[aria-label="Thread anchor"]')!
      .querySelector("button, a, input, textarea"),
  ).toBeNull()
})

it("hydrates a deep-linked branch after a fresh mount and offers root navigation when it is unavailable", async () => {
  const request = api()
  vi.stubGlobal("fetch", request)
  app = mount(ConversationFixture, {
    target: document.body,
    props: { initialPath: `${root}?parent=answer`, onNavigate: vi.fn() },
  })
  await vi.waitFor(() =>
    expect(document.body.textContent).toContain("Nested reply"),
  )
  expect(document.body.textContent).not.toContain("Root question")
  const anchor = document.querySelector('[aria-label="Thread anchor"]')!
  expect(anchor.textContent).toContain("Answer Markdown")
  expect(anchor.textContent).toContain("Archived assistant")
  expect(anchor.querySelector("time")?.dateTime).toBe(answer.created_at)
  expect(anchor.querySelector('a[download="result.txt"]')).not.toBeNull()
  expect(anchor.querySelector(`a[href="${root}?parent=answer"]`)).toBeNull()
  expect(document.querySelector('[data-slot="feed"]')!.contains(anchor)).toBe(
    false,
  )
  expect(
    (document.getElementById("message") as HTMLTextAreaElement).placeholder,
  ).toBe("Write a reply")
  await unmount(app)
  app = undefined
  const base = api()
  request.mockImplementation(async (path: string, options?: RequestInit) =>
    path.includes("/events/branch")
      ? new Response(null, { status: 404 })
      : base(path, options),
  )
  const navigate = vi.fn()
  app = mount(ConversationFixture, {
    target: document.body,
    props: { initialPath: `${root}?parent=missing`, onNavigate: navigate },
  })
  await vi.waitFor(() =>
    expect(document.body.textContent).toContain("This branch is unavailable"),
  )
  expect(document.querySelector('[aria-label="Thread anchor"]')).toBeNull()
  expect(document.body.textContent).not.toContain("Send the first message")
  expect(document.body.textContent).not.toContain("Answer Markdown")
  editField("message", "Draft while unavailable")
  expect(replyButton().disabled).toBe(true)
  backLink().click()
  expect(navigate).toHaveBeenLastCalledWith(root)
  await tick()
})
