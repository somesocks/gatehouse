import { afterEach, expect, it, vi } from "vitest"
import { clearAccessToken } from "../../../app/api"
import { createAgentProviderFormController } from "./agent-provider-form-controller.svelte"
afterEach(() => {
  clearAccessToken()
  vi.unstubAllGlobals()
})
it("omits hidden connection/credential fields when switching to built-in", async () => {
  const provider = {
    id: "apr_test",
    alias: "test",
    protocol: "openai-responses",
    endpoint: "https://example.test/v1/responses",
    keychain: { id: "default", version: 1 },
    enabled: true,
    revision: 1,
    credential_configured: true,
  }
  const request = vi.fn(
    async (url: RequestInfo | URL, options?: RequestInit) =>
      new Response(
        JSON.stringify(
          options?.method === "PATCH"
            ? { ...provider, protocol: "builtin" }
            : String(url).endsWith("keychains")
              ? []
              : provider,
        ),
        { status: 200 },
      ),
  )
  vi.stubGlobal("fetch", request)
  const controller = createAgentProviderFormController({
    onAuthenticationLost: vi.fn(),
    onSystemAccessChange: vi.fn(),
  })
  await controller.load(provider.id)
  await controller.beginEdit()
  controller.state.form.protocol = "builtin"
  await expect(controller.update()).resolves.toBe(true)
  const body = JSON.parse(
    request.mock.calls.find(([, options]) => options?.method === "PATCH")?.[1]
      ?.body as string,
  )
  expect(body).toMatchObject({
    alias: "test",
    protocol: "builtin",
    enabled: true,
    expected_revision: 1,
  })
  expect(body).not.toHaveProperty("api_key")
  expect(body).not.toHaveProperty("keychain")
  expect(body).not.toHaveProperty("endpoint")
})

it("creates a disconnected Codex provider without submitting a stale API key or endpoint", async () => {
  const fetch = vi.fn(
    async (_url: RequestInfo | URL, _options?: RequestInit) =>
      new Response(
        JSON.stringify({
          id: "apr_codex",
          alias: "chatgpt",
          revision: 1,
          protocol: "openai-codex",
          credential_configured: false,
          enabled: true,
        }),
      ),
  )
  vi.stubGlobal("fetch", fetch)
  const controller = createAgentProviderFormController({
    onAuthenticationLost: vi.fn(),
    onSystemAccessChange: vi.fn(),
  })
  Object.assign(controller.state.form, {
    alias: "chatgpt",
    protocol: "openai-codex",
    keychain: "default",
    endpoint: "https://old.test",
    apiKey: "OLD_SECRET",
  })
  const result = await controller.create()
  expect(result?.credential_configured).toBe(false)
  const body = JSON.parse(fetch.mock.calls[0][1]?.body as string)
  expect(body).toMatchObject({ protocol: "openai-codex", keychain: "default" })
  expect(body).not.toHaveProperty("api_key")
  expect(body).not.toHaveProperty("endpoint")
  controller.stop()
})

it("disconnects with the current revision and removes account metadata from its snapshot", async () => {
  const connected = {
    id: "apr_codex",
    alias: "chatgpt",
    revision: 3,
    protocol: "openai-codex",
    credential_configured: true,
    enabled: true,
    account: {
      id: "account",
      email: "user@example.com",
      expires_at: "2026-10-15T13:00:00Z",
    },
  }
  const fetch = vi.fn(
    async (_url: RequestInfo | URL, options?: RequestInit) =>
      new Response(
        JSON.stringify(
          options?.method === "POST"
            ? {
                ...connected,
                revision: 4,
                credential_configured: false,
                account: undefined,
              }
            : connected,
        ),
      ),
  )
  vi.stubGlobal("fetch", fetch)
  const controller = createAgentProviderFormController({
    onAuthenticationLost: vi.fn(),
    onSystemAccessChange: vi.fn(),
  })
  await controller.load(connected.id)
  await controller.disconnect()
  expect(fetch.mock.calls[1][0]).toBe(
    "/api/v1/system/agent-providers/apr_codex/disconnect",
  )
  expect(JSON.parse(fetch.mock.calls[1][1]?.body as string)).toEqual({
    expected_revision: 3,
  })
  expect(controller.state.provider?.revision).toBe(4)
  expect(controller.state.provider?.credential_configured).toBe(false)
  expect(controller.state.provider?.account).toBeUndefined()
  controller.stop()
})
