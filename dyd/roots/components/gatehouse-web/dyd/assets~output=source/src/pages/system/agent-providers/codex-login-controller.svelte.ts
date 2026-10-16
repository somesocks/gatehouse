import {
  startCodexLogin,
  pollCodexLogin,
  fetchAgentProvider,
  type CodexLoginBinding,
  type SystemAgentProvider,
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
  providerID,
  onAuthenticationLost,
  onSystemAccessChange,
}: {
  providerID: string
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const lifecycle = createLoadState()
  let disposed = false
  let pollTimer: ReturnType<typeof setTimeout> | undefined
  let expiryTimer: ReturnType<typeof setTimeout> | undefined
  let expiresAt = 0
  let binding: CodexLoginBinding | null = null
  const state = $state({
    phase: "idle" as Phase,
    attempt: null as CodexLoginAttempt | null,
    result: null as CodexLoginSuccess | null,
    provider: null as SystemAgentProvider | null,
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
    binding = null
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
    if (response.status === 409)
      return new Error(
        "This provider changed during sign-in. Review its connection before trying again.",
      )
    if (response.status === 404)
      return new Error(
        "The provider could not be found. Open the connection page from an existing OpenAI Codex provider.",
      )
    if (response.status === 503)
      return new Error(
        "The provider’s keychain is unavailable. Check its configuration and try again.",
      )
    if (response.status === 500)
      return new Error(
        "The account credentials could not be saved. Check the provider’s status before trying again.",
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
    binding = null
    state.remainingMinutes = null
    state.phase = "starting"
    const request = lifecycle.begin()
    try {
      if (!providerID)
        throw new Error("Choose an OpenAI Codex provider before connecting.")
      const providerResponse = await fetchAgentProvider(
        providerID,
        request.signal,
      )
      if (!request.current() || denied(providerResponse)) return
      if (!providerResponse.ok) throw await responseFailure(providerResponse)
      const provider = (await providerResponse.json()) as SystemAgentProvider
      if (!request.current()) return
      if (
        provider.id !== providerID ||
        provider.protocol !== "openai-codex" ||
        !Number.isInteger(provider.revision) ||
        provider.revision < 1 ||
        !provider.keychain
      ) {
        throw new Error("This provider is not configured for ChatGPT login.")
      }
      request.commit(() => {
        state.provider = provider
        binding = {
          provider: provider.id,
          expected_revision: provider.revision,
        }
      })
      const response = await startCodexLogin(binding!, request.signal)
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
    const currentBinding = binding
    if (
      disposed ||
      state.phase !== "pending" ||
      attempt === null ||
      currentBinding === null ||
      state.busy
    )
      return
    if (Date.now() >= expiresAt) {
      end("expired")
      return
    }
    const request = lifecycle.begin()
    try {
      const response = await pollCodexLogin(
        attempt,
        currentBinding,
        request.signal,
      )
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
        (result.expires_at === undefined ||
          Number.isFinite(Date.parse(result.expires_at))) &&
        result.provider?.id === providerID &&
        result.provider.protocol === "openai-codex" &&
        result.provider.credential_configured
      ) {
        request.commit(() => {
          clearTimers()
          state.attempt = null
          state.remainingMinutes = null
          state.result = result
          state.provider = result.provider
          binding = null
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
        let diagnostic = failure(error)
        // The exchange/save can have succeeded even if its response was lost.
        // Re-read authoritative status; never silently retry a one-time code or
        // claim that this attempt installed somebody else's concurrent session.
        try {
          const response = await fetchAgentProvider(providerID, request.signal)
          if (!request.current() || denied(response)) return
          if (response.ok) {
            const provider = (await response.json()) as SystemAgentProvider
            if (!request.current()) return
            if (provider.id === providerID) {
              state.provider = provider
              if (provider.revision !== currentBinding.expected_revision)
                diagnostic = new Error(
                  "This provider changed during sign-in. Review its connection before trying again.",
                )
            }
          }
        } catch {
          /* Retain the original error if status cannot be recovered. */
        }
        if (!request.current()) return
        state.attempt = null
        binding = null
        state.remainingMinutes = null
        state.phase = "failed"
        request.fail(diagnostic)
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
