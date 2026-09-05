import { afterEach, describe, expect, it, vi } from "vitest"
import { createActivityClient } from "./activity"

afterEach(() => {
  vi.useRealTimers()
})

describe("activity client", () => {
  it("sends topic checkpoints and maps the response", async () => {
    vi.useFakeTimers()
    const request = vi.fn(async () => Response.json({ topics: [{ name: "access", topic: "prn_1", events: ["system_grant.update"], cursor: { id: "act_1" } }] }))
    const client = createActivityClient({ onAuthenticationLost: () => {}, fetch: request })
    client.subscribe([{ name: "access", topic: "prn_1", events: ["system_grant.update"] }], async () => {})

    await client.poll()

    expect(request).toHaveBeenCalledWith("/api/v1/activity", expect.objectContaining({ method: "POST", credentials: "same-origin" }))
    client.dispose()
  })

  it("reports authentication loss", async () => {
    vi.useFakeTimers()
    const onAuthenticationLost = vi.fn()
    const client = createActivityClient({ onAuthenticationLost, fetch: async () => new Response(null, { status: 401 }) })
    client.subscribe([{ name: "access", topic: "prn_1", events: ["system_grant.update"] }], async () => {})

    await client.poll()

    expect(onAuthenticationLost).toHaveBeenCalledOnce()
    client.dispose()
  })
})
