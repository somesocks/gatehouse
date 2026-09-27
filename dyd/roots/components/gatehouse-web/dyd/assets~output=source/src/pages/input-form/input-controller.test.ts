import { describe, expect, it } from "vitest"
import { patchDraft } from "./form-data"
import type { InputTransport } from "./input-api"
import { createInputFormController } from "./input-controller.svelte"

const descriptor = {
  version: 1,
  type: "form",
  title: "Review",
  fields: [
    { id: "name", label: "Name", type: "text" },
    { id: "approved", label: "Approved", type: "boolean" },
    { id: "profile", label: "Profile", type: "object", optional: true, fields: [
      { id: "city", label: "City", type: "text" },
    ] },
  ],
}

function sharedInput(): { transport: () => InputTransport; snapshot: () => { draft: Record<string, unknown>; terminal: string | null } } {
  let draft: Record<string, unknown> = {}
  let terminal: string | null = null
  return {
    transport: () => ({
      read: async () => terminal === null
        ? new Response(JSON.stringify({ form: descriptor, draft, updated_at: null }), { status: 200 })
        : new Response(null, { status: 409 }),
      patch: async (body: string) => {
        if (terminal !== null) return new Response(null, { status: 409 })
        const operation = JSON.parse(body) as { op: string; path: string[]; value?: unknown }
        draft = patchDraft(draft, operation.path, operation.value, operation.op === "remove")
        return new Response(null, { status: 204 })
      },
      terminal: async (action: "submit" | "cancel") => {
        if (terminal !== null) return new Response(null, { status: 409 })
        if (action === "submit" && typeof draft.name !== "string")
          return new Response("name is required", { status: 422 })
        terminal = action
        return new Response(JSON.stringify({ event_id: "sev_result", kind: action }), { status: 202 })
      },
    }),
    snapshot: () => ({ draft, terminal }),
  }
}

describe("shared input form controller", () => {
  it("saves distinct fields, snapshots the stored draft, and blocks a second responder", async () => {
    const server = sharedInput()
    const alice = createInputFormController(server.transport())
    const bob = createInputFormController(server.transport())
    await Promise.all([alice.load(), bob.load()])
    expect(alice.state.status).toBe("ready")
    await alice.set(["name"], "Ada")
    await bob.set(["profile", "city"], "Paris")
    expect(alice.state.draft).toEqual({ name: "Ada" })
    expect(server.snapshot().draft).toEqual({ name: "Ada", profile: { city: "Paris" } })
    await alice.submit()
    expect(server.snapshot()).toEqual({
      draft: { name: "Ada", approved: false, profile: { city: "Paris" } },
      terminal: "submit",
    })
    expect(alice.state.status).toBe("submitted")
    await bob.submit()
    expect(bob.state.status).toBe("resolved")
  })

  it("lets a responder cancel an incomplete draft", async () => {
    const server = sharedInput()
    const user = createInputFormController(server.transport())
    await user.load()
    await user.cancel()
    expect(user.state.status).toBe("cancelled")
    expect(server.snapshot().terminal).toBe("cancel")
  })

  it("requires a shared-draft reload after a failed save", async () => {
    const server = sharedInput()
    const transport = server.transport()
    const patch = transport.patch
    let failOnce = true
    transport.patch = async (body: string) => {
      if (failOnce) {
        failOnce = false
        return new Response("invalid field", { status: 422 })
      }
      return patch(body)
    }
    const user = createInputFormController(transport)
    await user.load()
    expect(await user.set(["name"], "Ada")).toBe(false)
    await user.submit()
    expect(server.snapshot().terminal).toBeNull()
    expect(user.state.needsReload).toBe(true)
    await user.load()
    expect(user.state.draft).toEqual({})
    expect(await user.set(["name"], "Ada")).toBe(true)
    await user.submit()
    expect(user.state.status).toBe("submitted")
  })

  it("refreshes a shared draft without showing loading or replacing an unchanged form", async () => {
    const server = sharedInput()
    const alice = createInputFormController(server.transport())
    const bob = createInputFormController(server.transport())
    await Promise.all([alice.load(), bob.load()])
    const form = alice.state.form
    const draft = alice.state.draft
    await alice.refresh()
    expect(alice.state.status).toBe("ready")
    expect(alice.state.form).toBe(form)
    expect(alice.state.draft).toBe(draft)
    await bob.set(["name"], "Bob")
    await alice.refresh()
    expect(alice.state.status).toBe("ready")
    expect(alice.state.form).toBe(form)
    expect(alice.state.draft).toEqual({ name: "Bob" })
    await bob.cancel()
    await alice.refresh()
    expect(alice.state.status).toBe("resolved")
  })

  it("does not overwrite local edits made during a background read", async () => {
    const server = sharedInput()
    const transport = server.transport()
    const originalRead = transport.read
    const user = createInputFormController(transport)
    await user.load()
    let finishRead!: (value: Response) => void
    transport.read = () => new Promise<Response>((resolve) => { finishRead = resolve })
    const refreshing = user.refresh()
    await Promise.resolve()
    const saved = user.set(["name"], "Local")
    finishRead(await originalRead())
    await Promise.all([refreshing, saved])
    expect(user.state.status).toBe("ready")
    expect(user.state.draft).toEqual({ name: "Local" })
  })
})
