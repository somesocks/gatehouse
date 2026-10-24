import { readFileSync } from "node:fs"
import { parse } from "svelte/compiler"
import { describe, expect, it, vi } from "vitest"
import { clientCode } from "../testing/client-module"

// Exercise the real action dialog script with controlled prop updates and saves.
async function actionDialog() {
  const source = readFileSync(
    new URL("./SessionParticipantActionDialog.svelte", import.meta.url),
    "utf8",
  )
  const ast = parse(source, { modern: true })
  const props = ast.instance!.content.body.find(
    (node: any) =>
      node.type === "VariableDeclaration" &&
      node.declarations[0].init?.callee?.name === "$props",
  )!
  const body = source.slice(props.end, ast.instance!.content.end)
  const code = clientCode(`
    import { untrack, flushSync } from "svelte";
    export { flushSync };
    export function editor(initial, callbacks, mode = "role") {
      let grant = $state(initial);
      const canManage = true, saving = false;
      const { onUpdate, onRemove, onReload, onClose } = callbacks;
      let state;
      const dispose = $effect.root(() => {
        ${body}
        state = {
          get draft() { return draft; },
          get revision() { return snapshot.revision; },
          select(role) { draft = role; },
          refresh(next) { grant = next; },
          save, reload,
        };
      });
      return { state, dispose };
    }
  `)
  return await import(
    /* @vite-ignore */ `data:text/javascript;base64,${Buffer.from(code).toString("base64")}`
  )
}

const grant = { id: "grant", role: "guest", revision: 1, enabled: true }
function callbacks() {
  return {
    onUpdate: vi.fn(async () => false),
    onRemove: vi.fn(async () => false),
    onReload: vi.fn(async () => ({
      ...grant,
      role: "member",
      revision: 2,
    })),
    onClose: vi.fn(),
  }
}

describe("participant action dialogs", () => {
  it("keeps a role draft and its opening revision until explicitly reloaded", async () => {
    const { editor, flushSync } = await actionDialog()
    const actions = callbacks()
    const { state, dispose } = editor(grant, actions)
    try {
      flushSync()
      expect(actions.onUpdate).not.toHaveBeenCalled()
      state.select("admin")
      state.refresh({ ...grant, role: "member", revision: 2 })
      flushSync()
      expect(state.draft).toBe("admin")
      expect(state.revision).toBe(1)
      await state.save()
      expect(actions.onUpdate).toHaveBeenCalledWith(
        expect.objectContaining({ revision: 1 }),
        "admin",
      )
      expect(actions.onClose).not.toHaveBeenCalled()
      await state.reload()
      expect(state.draft).toBe("member")
      expect(state.revision).toBe(2)
      state.select("admin")
      actions.onUpdate.mockResolvedValueOnce(true)
      await state.save()
      expect(actions.onClose).toHaveBeenCalledWith(true)
    } finally {
      dispose()
    }
  })

  it("does not remove on opening or cancellation and requires an explicit confirmation", async () => {
    const { editor, flushSync } = await actionDialog()
    const actions = callbacks()
    const cancelled = editor(grant, actions, "remove")
    flushSync()
    cancelled.dispose()
    expect(actions.onRemove).not.toHaveBeenCalled()
    const confirmed = editor(grant, actions, "remove")
    try {
      flushSync()
      confirmed.state.refresh({ ...grant, revision: 2 })
      actions.onRemove.mockResolvedValueOnce(true)
      await confirmed.state.save()
      expect(actions.onRemove).toHaveBeenCalledTimes(1)
      expect(actions.onRemove).toHaveBeenCalledWith(
        expect.objectContaining({ id: "grant", revision: 1 }),
      )
      expect(actions.onClose).toHaveBeenCalledWith(true)
      expect(actions.onUpdate).not.toHaveBeenCalled()
    } finally {
      confirmed.dispose()
    }
  })

  it("does not close a replacement dialog after a cancelled confirmation finishes", async () => {
    const { editor, flushSync } = await actionDialog()
    const actions = callbacks()
    let finish!: (saved: boolean) => void
    actions.onRemove.mockImplementationOnce(
      async () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const { state, dispose } = editor(grant, actions, "remove")
    flushSync()
    const pending = state.save()
    dispose()
    finish(true)
    await pending
    expect(actions.onClose).not.toHaveBeenCalled()
  })
})
