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
    base_url: "https://example.test/v1",
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
  expect(body).not.toHaveProperty("base_url")
})
