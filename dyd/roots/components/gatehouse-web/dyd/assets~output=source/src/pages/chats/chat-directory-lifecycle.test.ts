import { readFileSync } from "node:fs"
import { createRequire } from "node:module"
import { dirname, join } from "node:path"
import { pathToFileURL } from "node:url"
import { compileModule, parse } from "svelte/compiler"
import { afterEach, describe, expect, it, vi } from "vitest"
import { createChatCollectionController } from "./chat-collection-controller.svelte"
import { createProjectCollectionController } from "../projects/project-collection-controller.svelte"
import type { ActivityRefresh } from "../../app/activity"

afterEach(() => vi.unstubAllGlobals())

// Run the page's actual subscription with Svelte's client reactivity, without
// mounting a browser or substituting a mock dependency-tracking implementation.
async function pageSubscription(page = "./ChatCollectionPage.svelte") {
  const source = readFileSync(new URL(page, import.meta.url), "utf8")
  const ast = parse(source, { modern: true })
  const statement = ast.instance!.content.body.find(
    (node: any) =>
      node.type === "ExpressionStatement" &&
      node.expression.callee?.name === "$effect",
  ) as any
  const callback = statement.expression.arguments[0]
  const body = source.slice(callback.start, callback.end)
  const require = createRequire(import.meta.url)
  const root = dirname(require.resolve("svelte/package.json"))
  const client = pathToFileURL(join(root, "src/index-client.js")).href
  const internal = pathToFileURL(
    join(root, "src/internal/client/index.js"),
  ).href
  const compiled = compileModule(
    `
    import { untrack, flushSync } from ${JSON.stringify(client)};
    export { flushSync };
    export function subscribe(controller, initialSignal) {
      let workspace = $state({ id: "workspace" });
      let search = $state("");
      let projectID = $state("");
      let signal = $state(initialSignal);
      const dispose = $effect.root(() => { $effect(${body}); });
      return {
        project(id) { projectID = id; },
        search(next) { search = next; },
        signal(next) { signal = next; },
        dispose,
      };
    }
  `,
    { filename: "directory-lifecycle.svelte.ts", generate: "client" },
  )
  const code = compiled.js.code.replace(
    /(['"])svelte\/internal\/client\1/g,
    JSON.stringify(internal),
  )
  return await import(
    /* @vite-ignore */ `data:text/javascript;base64,${Buffer.from(code).toString("base64")}`
  )
}

describe("workspace directory route lifecycle", () => {
  it("reconciles sharing changes without losing the current filters and disposes the subscription", async () => {
    let refresh!: ActivityRefresh
    const unsubscribe = vi.fn()
    const activity = {
      subscribe: vi.fn((_selectors, callback) => {
        refresh = callback
        return unsubscribe
      }),
      poll: vi.fn(async () => {}),
      dispose: vi.fn(),
    }
    const request = vi.fn(async () =>
      Response.json({ sessions: [{ id: "initial-chat" }] }),
    )
    vi.stubGlobal("fetch", request)
    const controller = createChatCollectionController({
      onAuthenticationLost: vi.fn(),
      activity,
    })
    const stop = controller.start(
      "workspace",
      "design",
      new AbortController().signal,
      "project",
    )
    await vi.waitFor(() =>
      expect(controller.state.chats[0]?.id).toBe("initial-chat"),
    )
    expect(activity.subscribe.mock.calls[0][0]).toEqual([
      expect.objectContaining({
        topic: "workspace",
        events: expect.arrayContaining(["session_grant.*", "group_member.*"]),
      }),
    ])
    request.mockResolvedValueOnce(
      Response.json({ sessions: [{ id: "newly-shared-chat" }] }),
    )
    await refresh({
      names: new Set(["chat-directory"]),
      signal: new AbortController().signal,
    })
    expect(controller.state.chats[0]?.id).toBe("newly-shared-chat")
    const path = new URL(
      (request.mock.calls.at(-1) as unknown as [string])[0],
      "http://localhost",
    )
    expect(path.searchParams.get("name")).toBe("design")
    expect(path.searchParams.get("project")).toBe("project")
    stop()
    expect(unsubscribe).toHaveBeenCalledTimes(1)
  })
  it("restarts the filtered request when the parent replaces an aborted route signal", async () => {
    const fetch = vi.fn(async (path: string, init: RequestInit) => {
      if (init.signal?.aborted)
        throw new DOMException("Route changed", "AbortError")
      const project = new URL(path, "http://localhost").searchParams.get(
        "project",
      )
      return Response.json({
        sessions: [
          {
            id: project ? "project-chat" : "workspace-chat",
            created_at: "2026-10-11",
          },
        ],
      })
    })
    vi.stubGlobal("fetch", fetch)
    const controller = createChatCollectionController({
      onAuthenticationLost: vi.fn(),
    })
    const lifecycle = await pageSubscription()
    const oldRoute = new AbortController()
    const subscription = lifecycle.subscribe(controller, oldRoute.signal)
    try {
      lifecycle.flushSync()
      await vi.waitFor(() =>
        expect(controller.state.chats[0]?.id).toBe("workspace-chat"),
      )
      // Route props can update before the replacement signal reaches the child.
      oldRoute.abort()
      subscription.project("project")
      lifecycle.flushSync()
      const newRoute = new AbortController()
      subscription.signal(newRoute.signal)
      lifecycle.flushSync()
      await vi.waitFor(() =>
        expect(controller.state.chats[0]?.id).toBe("project-chat"),
      )
      const requestSignal = fetch.mock.calls.at(-1)?.[1].signal
      expect(requestSignal?.aborted).toBe(false)
      newRoute.abort()
      expect(requestSignal?.aborted).toBe(true)
      expect(controller.state.error).toBe("")
    } finally {
      subscription.dispose()
      controller.stop()
    }
  })
  it("restarts project-name search when its route signal is replaced", async () => {
    const fetch = vi.fn(async (path: string, init: RequestInit) => {
      if (init.signal?.aborted)
        throw new DOMException("Route changed", "AbortError")
      const name = new URL(path, "http://localhost").searchParams.get("name")
      return Response.json({
        projects: [
          {
            id: name ? "matching-project" : "initial-project",
            created_at: "2026-10-11",
          },
        ],
      })
    })
    vi.stubGlobal("fetch", fetch)
    const controller = createProjectCollectionController({
      onAuthenticationLost: vi.fn(),
    })
    const lifecycle = await pageSubscription(
      "../projects/ProjectCollectionPage.svelte",
    )
    const oldRoute = new AbortController()
    const subscription = lifecycle.subscribe(controller, oldRoute.signal)
    try {
      lifecycle.flushSync()
      await vi.waitFor(() =>
        expect(controller.state.projects[0]?.id).toBe("initial-project"),
      )
      oldRoute.abort()
      subscription.search("matching")
      lifecycle.flushSync()
      const newRoute = new AbortController()
      subscription.signal(newRoute.signal)
      lifecycle.flushSync()
      await vi.waitFor(() =>
        expect(controller.state.projects[0]?.id).toBe("matching-project"),
      )
      const requestSignal = fetch.mock.calls.at(-1)?.[1].signal
      expect(requestSignal?.aborted).toBe(false)
      newRoute.abort()
      expect(requestSignal?.aborted).toBe(true)
    } finally {
      subscription.dispose()
      controller.stop()
    }
  })
})
