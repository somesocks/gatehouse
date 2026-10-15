import { render } from "svelte/server"
import { expect, it } from "vitest"
import CodexLoginCard from "./CodexLoginCard.svelte"
import type { createCodexLoginController } from "./codex-login-controller.svelte"

type State = ReturnType<typeof createCodexLoginController>["state"]
const attempt = {
  device_auth_id: "private-device-id",
  user_code: "ABCD-EFGH",
  verification_uri: "https://auth.openai.com/codex/device",
  expires_at: "2026-10-15T12:15:00Z",
  poll_after_ms: 5000,
}

function card(phase: State["phase"], options: Partial<State> = {}) {
  const state: State = {
    phase,
    attempt: phase === "pending" ? attempt : null,
    result: null,
    error: "",
    busy: false,
    remainingMinutes: 12,
    ...options,
  }
  const noop = () => {}
  return render(CodexLoginCard, {
    props: {
      state,
      onstart: noop,
      oncopy: noop,
      oncancel: noop,
      onclose: noop,
    },
  }).body
}

it("keeps copy as an accessible icon beside selectable code, with steady waiting controls", () => {
  const waiting = card("pending")
  const polling = card("pending", { busy: true })
  expect(polling).toBe(waiting)
  expect(waiting).toContain("ABCD-EFGH")
  expect(waiting).toContain('aria-label="Copy code"')
  expect(waiting).toContain('class="min-w-0 select-all')
  expect(waiting).not.toContain("<input")
  expect(waiting).not.toContain(">Copy code<")
  expect(waiting).not.toContain("copied")
  expect(waiting).toContain('target="_blank"')
  expect(waiting).toContain("Expires in 12 minutes")
  expect(waiting).not.toContain(attempt.device_auth_id)
  const live = waiting.match(/role="status"[^>]*>([\s\S]*?)<\/span>/)?.[1]
  expect(live).toContain("Waiting for approval.")
  expect(live).not.toContain("Expires")
})

it("keeps an accessible disabled submitting button alongside cancellation", () => {
  const body = card("starting", { busy: true })
  expect(body).toMatch(/<button[^>]*disabled/)
  expect(body).toContain('aria-busy="true"')
  expect(body).toContain("Starting…")
  expect(body).toContain("Cancel")
  expect(body).not.toContain("Continue with ChatGPT")
})

it("shows success without internal identifiers or token-expiry details", () => {
  const body = card("succeeded", {
    result: {
      status: "succeeded",
      account: { id: "private-account-id", email: "user@example.com" },
      expires_at: "2026-10-15T13:00:00Z",
    },
  })
  expect(body).toContain("Login verified")
  expect(body).toContain("user@example.com")
  expect(body).toContain("Close tab")
  expect(body).not.toContain("private-account-id")
  expect(body).not.toContain("2026-10-15")
  expect(body).not.toContain("Connected")
  expect(body).not.toContain("ABCD-EFGH")
})

it.each([
  {
    phase: "failed",
    message: "Couldn’t complete sign-in.",
    action: "Try again",
  },
  {
    phase: "expired",
    message: "This code has expired.",
    action: "Start again",
  },
  { phase: "cancelled", message: "Sign-in cancelled.", action: "Start again" },
] as const)(
  "replaces the code with useful recovery actions after $phase",
  ({ phase, message, action }) => {
    const body = card(phase, {
      error: phase === "failed" ? "Check your connection and try again." : "",
    })
    expect(body).toContain(message)
    expect(body).toContain(action)
    expect(body).toContain("Close tab")
    expect(body).not.toContain("ABCD-EFGH")
    expect(body).not.toContain("Open OpenAI")
    if (phase === "failed") {
      expect(body).toContain('role="alert"')
      expect(body).toContain("Check your connection")
    }
  },
)
