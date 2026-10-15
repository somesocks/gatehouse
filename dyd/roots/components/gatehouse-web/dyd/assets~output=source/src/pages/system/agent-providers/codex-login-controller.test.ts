import { afterEach, beforeEach, expect, it, vi } from "vitest"
import { clearAccessToken } from "../../../app/api"
import { createCodexLoginController } from "./codex-login-controller.svelte"

const now = new Date("2026-10-15T12:00:00Z")
const provider = {
  id: "apr_test",
  alias: "chatgpt",
  revision: 1,
  protocol: "openai-codex",
  keychain: { id: "default", version: 1 },
  credential_configured: false,
  enabled: true,
}
const attempt = {
  device_auth_id: "device",
  user_code: "ABCD-EFGH",
  verification_uri: "https://auth.openai.com/codex/device",
  expires_at: "2026-10-15T12:15:00Z",
  poll_after_ms: 7000,
}
const success = {
  status: "succeeded",
  account: { id: "account", email: "user@example.com" },
  expires_at: "2026-10-15T13:00:00Z",
  provider: { ...provider, revision: 2, credential_configured: true },
}
const json = (value: unknown) => new Response(JSON.stringify(value))
function stubFetch(fetch: ReturnType<typeof vi.fn>, snapshot = provider) {
  vi.stubGlobal("fetch", (input: RequestInfo | URL, options?: RequestInit) =>
    String(input) === "/api/v1/system/agent-providers/apr_test"
      ? Promise.resolve(json(snapshot))
      : fetch(input, options),
  )
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((accept) => {
    resolve = accept
  })
  return { promise, resolve }
}
function controller() {
  const onAuthenticationLost = vi.fn()
  const onSystemAccessChange = vi.fn()
  return {
    ...createCodexLoginController({
      providerID: provider.id,
      onAuthenticationLost,
      onSystemAccessChange,
    }),
    onAuthenticationLost,
    onSystemAccessChange,
  }
}
beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(now)
})
afterEach(() => {
  clearAccessToken()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

it("starts and polls through Gatehouse, respecting the original interval until success", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(json(attempt))
    .mockResolvedValueOnce(json({ status: "pending", poll_after_ms: 5000 }))
    .mockResolvedValueOnce(json(success))
  stubFetch(fetch)
  const login = controller()
  await login.start()
  expect(login.state.phase).toBe("pending")
  expect(login.state.remainingMinutes).toBe(15)
  expect(fetch.mock.calls[0][0]).toBe(
    "/api/v1/system/agent-providers/openai-codex/login/start",
  )
  expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({
    provider: provider.id,
    expected_revision: 1,
  })
  await vi.advanceTimersByTimeAsync(6999)
  expect(fetch).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(1)
  expect(fetch.mock.calls[1][0]).toBe(
    "/api/v1/system/agent-providers/openai-codex/login/poll",
  )
  expect(JSON.parse(fetch.mock.calls[1][1].body)).toEqual({
    provider: provider.id,
    expected_revision: 1,
    device_auth_id: "device",
    user_code: "ABCD-EFGH",
  })
  await vi.advanceTimersByTimeAsync(6999)
  expect(fetch).toHaveBeenCalledTimes(2)
  await vi.advanceTimersByTimeAsync(1)
  expect(login.state.phase).toBe("succeeded")
  expect(login.state.result).toEqual(success)
  expect(login.state.attempt).toBeNull()
  await vi.advanceTimersByTimeAsync(900_000)
  expect(fetch).toHaveBeenCalledTimes(3)
  expect(login.state.phase).toBe("succeeded")
  login.stop()
})

it("does not overlap polling requests and aborts the current request on cancellation", async () => {
  const pending = deferred<Response>()
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(json(attempt))
    .mockReturnValueOnce(pending.promise)
  stubFetch(fetch)
  const login = controller()
  await login.start()
  await vi.advanceTimersByTimeAsync(7000)
  await vi.advanceTimersByTimeAsync(20_000)
  expect(fetch).toHaveBeenCalledTimes(2)
  const signal = fetch.mock.calls[1][1].signal as AbortSignal
  login.cancel()
  expect(signal.aborted).toBe(true)
  pending.resolve(json(success))
  await vi.advanceTimersByTimeAsync(0)
  expect(login.state.phase).toBe("cancelled")
  expect(login.state.result).toBeNull()
  expect(login.state.attempt).toBeNull()
  expect(vi.getTimerCount()).toBe(0)
})

it("ignores a success whose body finishes decoding after cancellation", async () => {
  const decoded = deferred<unknown>()
  const response = new Response()
  vi.spyOn(response, "json").mockReturnValue(decoded.promise)
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(json(attempt))
    .mockResolvedValueOnce(response)
  stubFetch(fetch)
  const login = controller()
  await login.start()
  await vi.advanceTimersByTimeAsync(7000)
  login.cancel()
  decoded.resolve(success)
  await vi.advanceTimersByTimeAsync(0)
  expect(login.state.phase).toBe("cancelled")
  expect(login.state.result).toBeNull()
})

it("expires and aborts an in-flight poll without publishing a late access failure", async () => {
  const pending = deferred<Response>()
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(
      json({ ...attempt, expires_at: "2026-10-15T12:00:10Z" }),
    )
    .mockReturnValueOnce(pending.promise)
  stubFetch(fetch)
  const login = controller()
  await login.start()
  await vi.advanceTimersByTimeAsync(10_000)
  expect(login.state.phase).toBe("expired")
  expect((fetch.mock.calls[1][1].signal as AbortSignal).aborted).toBe(true)
  pending.resolve(new Response("forbidden", { status: 403 }))
  await vi.advanceTimersByTimeAsync(0)
  expect(login.onSystemAccessChange).not.toHaveBeenCalled()
  expect(login.state.phase).toBe("expired")
  expect(vi.getTimerCount()).toBe(0)
})

it("disposal aborts start and prevents late authentication callbacks or restarting", async () => {
  const pending = deferred<Response>()
  const fetch = vi.fn().mockReturnValue(pending.promise)
  stubFetch(fetch)
  const login = controller()
  const starting = login.start()
  await vi.advanceTimersByTimeAsync(0)
  login.stop()
  expect((fetch.mock.calls[0][1].signal as AbortSignal).aborted).toBe(true)
  pending.resolve(new Response("unauthorized", { status: 401 }))
  await starting
  await login.start()
  expect(login.onAuthenticationLost).not.toHaveBeenCalled()
  expect(login.state.attempt).toBeNull()
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(vi.getTimerCount()).toBe(0)
})

it("reports a token-exchange failure, clears temporary state, and supports a fresh login", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(json(attempt))
    .mockResolvedValueOnce(
      new Response("OpenAI login failed. Start a new login and try again.", {
        status: 502,
      }),
    )
    .mockResolvedValueOnce(json(attempt))
  stubFetch(fetch)
  const login = controller()
  await login.start()
  await vi.advanceTimersByTimeAsync(7000)
  expect(login.state.phase).toBe("failed")
  expect(login.state.error).toContain("Please try again")
  expect(login.state.attempt).toBeNull()
  expect(vi.getTimerCount()).toBe(0)
  await login.start()
  expect(login.state.phase).toBe("pending")
  expect(login.state.error).toBe("")
  login.stop()
})

it.each([401, 403])(
  "stops polling on confirmed access loss (%s)",
  async (status) => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(json(attempt))
      .mockResolvedValueOnce(new Response("denied", { status }))
    stubFetch(fetch)
    const login = controller()
    await login.start()
    await vi.advanceTimersByTimeAsync(7000)
    if (status === 401)
      expect(login.onAuthenticationLost).toHaveBeenCalledOnce()
    else expect(login.onSystemAccessChange).toHaveBeenCalledWith("denied")
    expect(login.state.attempt).toBeNull()
    expect(vi.getTimerCount()).toBe(0)
  },
)

it("rejects an unexpected verification origin before displaying or polling it", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(
      json({ ...attempt, verification_uri: "https://other.test/codex/device" }),
    )
  stubFetch(fetch)
  const login = controller()
  await login.start()
  expect(login.state.phase).toBe("failed")
  expect(login.state.attempt).toBeNull()
  expect(vi.getTimerCount()).toBe(0)
})

it("updates relative expiry without changing the waiting phase or overlapping a slow poll", async () => {
  const pending = deferred<Response>()
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(json(attempt))
    .mockReturnValueOnce(pending.promise)
  stubFetch(fetch)
  const login = controller()
  await login.start()
  await vi.advanceTimersByTimeAsync(60_000)
  expect(login.state.remainingMinutes).toBe(14)
  expect(login.state.phase).toBe("pending")
  expect(login.state.busy).toBe(true)
  await vi.advanceTimersByTimeAsync(13 * 60_000)
  expect(login.state.remainingMinutes).toBe(1)
  expect(fetch).toHaveBeenCalledTimes(2)
  await vi.advanceTimersByTimeAsync(60_000)
  expect(login.state.phase).toBe("expired")
  expect(login.state.remainingMinutes).toBeNull()
  expect(vi.getTimerCount()).toBe(0)
  pending.resolve(json(success))
  await vi.advanceTimersByTimeAsync(0)
  expect(login.state.phase).toBe("expired")
})

it("prevents duplicate submissions while starting and cancels that request", async () => {
  const pending = deferred<Response>()
  const fetch = vi.fn().mockReturnValue(pending.promise)
  stubFetch(fetch)
  const login = controller()
  const starting = login.start()
  await login.start()
  await vi.advanceTimersByTimeAsync(0)
  expect(login.state.phase).toBe("starting")
  expect(login.state.busy).toBe(true)
  expect(fetch).toHaveBeenCalledOnce()
  login.cancel()
  pending.resolve(json(attempt))
  await starting
  expect(login.state.phase).toBe("cancelled")
  expect(login.state.attempt).toBeNull()
  expect(vi.getTimerCount()).toBe(0)
})

it("shows an expired state when a code is already expired on receipt", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValueOnce(json({ ...attempt, expires_at: now.toISOString() }))
  stubFetch(fetch)
  const login = controller()
  await login.start()
  expect(login.state.phase).toBe("expired")
  expect(login.state.error).toBe("")
  expect(login.state.attempt).toBeNull()
  expect(vi.getTimerCount()).toBe(0)
})

it.each([
  {
    response: () =>
      new Response("Device-code login is unavailable. Check your settings.", {
        status: 502,
      }),
    message: "ChatGPT security settings",
  },
  {
    response: () => new Response("timeout", { status: 504 }),
    message: "too long to respond",
  },
  {
    response: () =>
      new Response("<html>internal proxy details</html>", { status: 502 }),
    message: "Please try again",
  },
])(
  "gives an actionable failure without displaying raw response details ($message)",
  async ({ response, message }) => {
    stubFetch(vi.fn().mockResolvedValueOnce(response()))
    const login = controller()
    await login.start()
    expect(login.state.phase).toBe("failed")
    expect(login.state.error).toContain(message)
    expect(login.state.error).not.toContain("<html>")
    expect(vi.getTimerCount()).toBe(0)
  },
)

it("describes a network failure without exposing the transport exception", async () => {
  stubFetch(vi.fn().mockRejectedValueOnce(new TypeError("Failed to fetch")))
  const login = controller()
  await login.start()
  expect(login.state.phase).toBe("failed")
  expect(login.state.error).toBe("Check your connection and try again.")
})

it("recovers provider status after losing the final response without retrying the exchange", async () => {
  let reads = 0
  const post = vi
    .fn()
    .mockResolvedValueOnce(json(attempt))
    .mockRejectedValueOnce(new TypeError("lost response"))
  vi.stubGlobal("fetch", (input: RequestInfo | URL, options?: RequestInit) => {
    if (String(input) === "/api/v1/system/agent-providers/apr_test") {
      reads++
      return Promise.resolve(json(reads === 1 ? provider : success.provider))
    }
    return post(input, options)
  })
  const login = controller()
  await login.start()
  await vi.advanceTimersByTimeAsync(7000)
  expect(reads).toBe(2)
  expect(post).toHaveBeenCalledTimes(2)
  expect(login.state.provider?.credential_configured).toBe(true)
  expect(login.state.result).toBeNull()
  expect(login.state.error).toContain("Review its connection")
  expect(vi.getTimerCount()).toBe(0)
})

it("uses the latest provider revision on a new attempt after a conflict", async () => {
  let reads = 0
  const post = vi
    .fn()
    .mockResolvedValueOnce(json(attempt))
    .mockResolvedValueOnce(new Response("revision conflict", { status: 409 }))
    .mockResolvedValueOnce(json(attempt))
  vi.stubGlobal("fetch", (input: RequestInfo | URL, options?: RequestInit) => {
    if (String(input) === "/api/v1/system/agent-providers/apr_test") {
      reads++
      return Promise.resolve(
        json({ ...provider, revision: reads === 1 ? 1 : 3 }),
      )
    }
    return post(input, options)
  })
  const login = controller()
  await login.start()
  await vi.advanceTimersByTimeAsync(7000)
  expect(login.state.phase).toBe("failed")
  await login.start()
  expect(JSON.parse(post.mock.calls[2][1].body)).toEqual({
    provider: provider.id,
    expected_revision: 3,
  })
  login.stop()
})
