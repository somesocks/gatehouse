import { afterEach, expect, it, vi } from "vitest"
import { clearAccessToken } from "../../../app/api"
import { createStorageProviderFormController } from "./storage-provider-form-controller.svelte"
afterEach(() => {
  clearAccessToken()
  vi.unstubAllGlobals()
})

it("creates S3 storage without selecting or fetching a keychain", async () => {
  const request = vi.fn(
    async (_url: RequestInfo | URL, _options?: RequestInit) =>
      Response.json({ id: "stp_new" }),
  )
  vi.stubGlobal("fetch", request)
  const controller = createStorageProviderFormController({
    onAuthenticationLost: vi.fn(),
    onSystemAccessChange: vi.fn(),
  })
  Object.assign(controller.state.form, {
    alias: "new",
    protocol: "s3",
    endpoint: "https://s3.example.test",
    region: "region",
    bucket: "bucket",
    accessKeyID: "access",
    secret: "secret",
  })
  expect(await controller.create()).toMatchObject({ id: "stp_new" })
  expect(request).toHaveBeenCalledOnce()
  const body = JSON.parse(request.mock.calls[0][1]?.body as string)
  expect(body).toMatchObject({ secret_access_key: "secret", protocol: "s3" })
  expect(body).not.toHaveProperty("keychain")
  controller.stop()
})
it("omits hidden S3 fields when switching to embedded storage", async () => {
  const provider = {
    id: "stp_test",
    alias: "test",
    protocol: "s3",
    endpoint: "https://s3.example.test",
    region: "test",
    bucket: "documents",
    access_key_id: "access",
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
            ? { ...provider, protocol: "embedded" }
            : String(url).endsWith("keychains")
              ? []
              : provider,
        ),
        { status: 200 },
      ),
  )
  vi.stubGlobal("fetch", request)
  const controller = createStorageProviderFormController({
    onAuthenticationLost: vi.fn(),
    onSystemAccessChange: vi.fn(),
  })
  await controller.load(provider.id)
  await controller.beginEdit()
  controller.state.form.protocol = "embedded"
  await expect(controller.update()).resolves.toBe(true)
  const body = JSON.parse(
    request.mock.calls.find(([, options]) => options?.method === "PATCH")?.[1]
      ?.body as string,
  )
  expect(body).toMatchObject({
    alias: "test",
    protocol: "embedded",
    enabled: true,
    expected_revision: 1,
  })
  for (const key of [
    "endpoint",
    "region",
    "bucket",
    "access_key_id",
    "keychain",
    "secret_access_key",
  ])
    expect(body).not.toHaveProperty(key)
})
