import { describe, expect, it, vi } from "vitest"
import { importClientModule } from "../testing/client-module"

async function create() {
  const { createConfirmation } = await importClientModule(
    new URL("./confirmation.svelte.ts", import.meta.url),
  )
  return createConfirmation()
}

describe("destructive confirmation lifecycle", () => {
  it("does not mutate when opened or cancelled", async () => {
    const confirmation = await create()
    const mutate = vi.fn()
    const request = {
      title: "Remove member?",
      description: "Remove Alice from the group?",
      confirmLabel: "Remove member",
      onConfirm: mutate,
    }
    confirmation.request(request)
    request.description = "A different target"
    expect(confirmation.state.request.description).toBe("Remove Alice from the group?")
    expect(mutate).not.toHaveBeenCalled()

    confirmation.cancel()
    await confirmation.confirm()
    expect(confirmation.state.request).toBeNull()
    expect(mutate).not.toHaveBeenCalled()
  })

  it("commits once and prevents dismissal or replacement while pending", async () => {
    const confirmation = await create()
    let finish!: () => void
    const mutate = vi.fn(() => new Promise<void>((resolve) => { finish = resolve }))
    confirmation.request({
      title: "Delete group?", description: "Delete Engineering?",
      confirmLabel: "Delete group", onConfirm: mutate,
    })
    const pending = confirmation.confirm()
    await confirmation.confirm()
    confirmation.cancel()
    confirmation.request({ title: "Other target", description: "Other", confirmLabel: "Delete", onConfirm: vi.fn() })
    expect(confirmation.state.pending).toBe(true)
    expect(confirmation.state.request.title).toBe("Delete group?")
    expect(mutate).toHaveBeenCalledOnce()
    finish()
    await pending
    expect(confirmation.state.request).toBeNull()
    expect(confirmation.state.pending).toBe(false)
  })

  it("retains the target and shows failed mutations until retried or cancelled", async () => {
    const confirmation = await create()
    const mutate = vi.fn().mockResolvedValueOnce("Revision conflict. Cancel and reopen.").mockResolvedValueOnce("")
    confirmation.request({ title: "Revoke access?", description: "Revoke Alice's grant?", confirmLabel: "Revoke", onConfirm: mutate })
    await confirmation.confirm()
    expect(confirmation.state.request.description).toBe("Revoke Alice's grant?")
    expect(confirmation.state.error).toBe("Revision conflict. Cancel and reopen.")
    expect(confirmation.state.pending).toBe(false)
    await confirmation.confirm()
    expect(mutate).toHaveBeenCalledTimes(2)
    expect(confirmation.state.request).toBeNull()
  })

  it("ignores late failures after the originating view is reset", async () => {
    const confirmation = await create()
    let fail!: (error: Error) => void
    confirmation.request({
      title: "Old view", description: "Old target", confirmLabel: "Delete",
      onConfirm: () => new Promise<void>((_resolve, reject) => { fail = reject }),
    })
    const pending = confirmation.confirm()
    confirmation.reset()
    confirmation.request({ title: "New view", description: "New target", confirmLabel: "Delete", onConfirm: vi.fn() })
    fail(new Error("Late failure"))
    await pending
    expect(confirmation.state.request.title).toBe("New view")
    expect(confirmation.state.error).toBe("")
    expect(confirmation.state.pending).toBe(false)
  })
})
