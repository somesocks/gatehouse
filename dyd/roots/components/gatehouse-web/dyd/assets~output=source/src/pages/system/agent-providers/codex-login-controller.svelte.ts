import {
  startCodexLogin,
  pollCodexLogin,
  type CodexLoginAttempt,
  type CodexLoginPoll,
  type CodexLoginSuccess,
} from "../../../app/system"
import { createLoadState } from "../../../utils/load-state.svelte"

type Phase =
  | "idle"
  | "starting"
  | "pending"
  | "succeeded"
  | "cancelled"
  | "expired"
  | "failed"

export function createCodexLoginController({
  onAuthenticationLost,
  onSystemAccessChange,
}: {
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const lifecycle = createLoadState()
  let disposed = false
  let pollTimer: ReturnType<typeof setTimeout> | undefined
  let expiryTimer: ReturnType<typeof setTimeout> | undefined
  let expiresAt = 0
  const state = $state({
    phase: "idle" as Phase,
    attempt: null as CodexLoginAttempt | null,
    result: null as CodexLoginSuccess | null,
    remainingMinutes: null as number | null,
    get error() {
      return lifecycle.state.error
    },
    get busy() {
      return lifecycle.state.pending
    },
  })

  function clearTimers() {
    clearTimeout(pollTimer)
    clearTimeout(expiryTimer)
    pollTimer = undefined
    expiryTimer = undefined
  }

  function end(phase: Phase) {
    clearTimers()
    lifecycle.stop()
    state.attempt = null
    state.remainingMinutes = null
    state.phase = phase
  }

  function schedulePoll(delay: number) {
    clearTimeout(pollTimer)
    pollTimer = setTimeout(() => void poll(), delay)
  }

  function validDelay(value: number): boolean {
    return Number.isFinite(value) && value >= 1000 && value <= 900_000
  }

  // Update only at minute boundaries. This local clock neither triggers polling
  // nor changes the phase announced by the card's live region.
  function updateExpiry() {
    const remaining = expiresAt - Date.now()
    if (remaining <= 0) {
      end("expired")
      return
    }
    state.remainingMinutes = Math.ceil(remaining / 60_000)
    expiryTimer = setTimeout(updateExpiry, remaining % 60_000 || 60_000)
  }

  async function responseFailure(response: Response): Promise<Error> {
    const message = await response.text()
    if (response.status === 504)
      return new Error("OpenAI took too long to respond. Please try again.")
    if (response.status === 429)
      return new Error(
        "Too many sign-in attempts. Wait a moment and try again.",
      )
    if (message.includes("Device-code login is unavailable.")) {
      return new Error(
        "Check that device-code login is enabled in your ChatGPT security settings, then try again.",
      )
    }
    return new Error("OpenAI couldn’t complete this sign-in. Please try again.")
  }

  function failure(error: unknown): Error {
    if (error instanceof TypeError)
      return new Error("Check your connection and try again.")
    if (error instanceof SyntaxError)
      return new Error(
        "The sign-in response could not be read. Please try again.",
      )
    return error instanceof Error ? error : new Error("Please try again.")
  }

  // Authorization callbacks and publication are always guarded by request
  // ownership, including after asynchronous body decoding.
  function denied(response: Response): boolean {
    if (response.status === 401) {
      end("failed")
      onAuthenticationLost()
      return true
    }
    if (response.status === 403) {
      end("failed")
      onSystemAccessChange("denied")
      return true
    }
    return false
  }

  async function start(): Promise<void> {
    if (disposed || state.busy || state.phase === "pending") return
    clearTimers()
    lifecycle.reset()
    state.attempt = null
    state.result = null
    state.remainingMinutes = null
    state.phase = "starting"
    const request = lifecycle.begin()
    try {
      const response = await startCodexLogin(request.signal)
      if (!request.current() || denied(response)) return
      if (!response.ok) throw await responseFailure(response)
      const attempt = (await response.json()) as CodexLoginAttempt
      if (!request.current()) return
      if (!attempt || typeof attempt !== "object")
        throw new Error(
          "The sign-in response could not be read. Please try again.",
        )
      const remaining = Date.parse(attempt.expires_at) - Date.now()
      if (
        typeof attempt.device_auth_id !== "string" ||
        !attempt.device_auth_id.trim() ||
        typeof attempt.user_code !== "string" ||
        !attempt.user_code.trim() ||
        attempt.verification_uri !== "https://auth.openai.com/codex/device" ||
        !validDelay(attempt.poll_after_ms) ||
        !Number.isFinite(remaining)
      )
        throw new Error("OpenAI returned an invalid login attempt. Try again.")
      request.commit(() => {
        if (remaining <= 0) {
          end("expired")
          return
        }
        state.attempt = attempt
        state.phase = "pending"
        expiresAt = Date.now() + Math.min(remaining, 900_000)
        updateExpiry()
        schedulePoll(attempt.poll_after_ms)
      })
    } catch (error) {
      if (request.current()) {
        state.phase = "failed"
        request.fail(failure(error))
      }
    } finally {
      request.finish()
    }
  }

  async function poll(): Promise<void> {
    const attempt = state.attempt
    if (disposed || state.phase !== "pending" || attempt === null || state.busy)
      return
    if (Date.now() >= expiresAt) {
      end("expired")
      return
    }
    const request = lifecycle.begin()
    try {
      const response = await pollCodexLogin(attempt, request.signal)
      if (!request.current() || denied(response)) return
      if (!response.ok) throw await responseFailure(response)
      const result = (await response.json()) as CodexLoginPoll
      if (!request.current()) return
      if (!result || typeof result !== "object")
        throw new Error(
          "The sign-in response could not be read. Please try again.",
        )
      if (result.status === "pending" && validDelay(result.poll_after_ms)) {
        request.commit(() =>
          schedulePoll(Math.max(attempt.poll_after_ms, result.poll_after_ms)),
        )
      } else if (
        result.status === "succeeded" &&
        typeof result.account?.id === "string" &&
        result.account.id.trim() &&
        Number.isFinite(Date.parse(result.expires_at))
      ) {
        request.commit(() => {
          clearTimers()
          state.attempt = null
          state.remainingMinutes = null
          state.result = result
          state.phase = "succeeded"
        })
      } else {
        throw new Error(
          "OpenAI returned an invalid login result. Start a new login and try again.",
        )
      }
    } catch (error) {
      if (request.current()) {
        clearTimers()
        state.attempt = null
        state.remainingMinutes = null
        state.phase = "failed"
        request.fail(failure(error))
      }
    } finally {
      request.finish()
    }
  }

  return {
    state,
    start,
    cancel() {
      lifecycle.reset()
      end("cancelled")
    },
    stop() {
      disposed = true
      end("idle")
      state.result = null
    },
  }
}
