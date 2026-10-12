import { readFileSync } from "node:fs"
import { parse } from "svelte/compiler"
import { afterEach, describe, expect, it, vi } from "vitest"
import {
  clientCode,
  clientEntry,
  importClientModule,
} from "../../testing/client-module"

afterEach(() => vi.unstubAllGlobals())

describe("Groups lifecycle", () => {
  it("keeps a submitted filter and advances pages without reactivating the route", async () => {
    const module = await importClientModule(
      new URL("./groups-controller.svelte.ts", import.meta.url),
    )
    const source = readFileSync(
      new URL("./GroupsPage.svelte", import.meta.url),
      "utf8",
    )
    const ast = parse(source, { modern: true })
    const effect = ast.instance!.content.body.find(
      (node: any) =>
        node.type === "ExpressionStatement" &&
        node.expression.callee?.name === "$effect",
    ) as any
    const callback = effect.expression.arguments[0]
    const compiled =
      clientCode(`import { untrack, flushSync } from ${JSON.stringify(clientEntry)}; export { flushSync };
      export function subscribe(controller, signal) {
        const workspace = { id: 'workspace' };
        return $effect.root(() => { $effect(${source.slice(callback.start, callback.end)}); });
      }`)
    const runtime = await import(
      /* @vite-ignore */ `data:text/javascript;base64,${Buffer.from(compiled).toString("base64")}`
    )
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string) => {
        const params = new URL(path, "http://localhost").searchParams
        const name = params.get("name") ?? ""
        const cursor = params.get("cursor") ?? ""
        return Response.json([{ id: cursor ? "second" : "first", name }], {
          headers: cursor ? {} : { "X-Next-Cursor": "next" },
        })
      }),
    )
    const activity = { subscribe: vi.fn(() => () => {}) }
    const controller = module.createGroupsController({
      activity,
      onAuthenticationLost: vi.fn(),
    })
    const signal = new AbortController().signal
    const dispose = runtime.subscribe(controller, signal)
    try {
      runtime.flushSync()
      await vi.waitFor(() => expect(controller.state.groups.length).toBe(1))
      controller.page.state.search = "members"
      await controller.page.search(() => controller.load("workspace", signal))
      runtime.flushSync()
      expect(controller.page.state.query).toBe("members")
      expect(controller.state.groups[0].name).toBe("members")
      await controller.page.next(() => controller.load("workspace", signal))
      runtime.flushSync()
      expect(controller.state.groups[0].id).toBe("second")
      expect(controller.page.state.history).toEqual([""])
      expect(activity.subscribe).toHaveBeenCalledTimes(1)
    } finally {
      dispose()
      controller.stop()
    }
  })
})
