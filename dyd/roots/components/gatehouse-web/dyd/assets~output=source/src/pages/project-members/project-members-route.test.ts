import { readFileSync } from "node:fs"
import { createSourceFile, isImportDeclaration, ScriptTarget } from "typescript"
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import { clientCode, clientEntry, clientModuleURL } from "../../testing/client-module"

afterEach(() => vi.unstubAllGlobals())

let createHarness: (runtime: any) => any
let fixture: any
beforeAll(async () => {
  const url = new URL("./ProjectMembersRoute.svelte", import.meta.url)
  const source = readFileSync(url, "utf8").match(/<script lang="ts">([\s\S]*?)<\/script>/)![1]
  const ast = createSourceFile(url.pathname, source, ScriptTarget.Latest, true)
  const imports = ast.statements.filter(isImportDeclaration)
  const kept = imports.filter((statement) =>
    !/ui|runtime\.svelte|ProjectMembersPage\.svelte/.test(statement.moduleSpecifier.getText(ast)),
  ).map((statement) => statement.getText(ast))
  let body = source
  for (const statement of [...imports].reverse())
    body = body.slice(0, statement.getStart(ast)) + body.slice(statement.end)
  body = body.replace(/const runtime = useRuntime\(\)/, "")
  const module = await import(/* @vite-ignore */ clientModuleURL(url,
    `${kept.join("\n")}\nexport function createHarness(runtime) { ${body}\n return { get project() { return project }, get error() { return resource.state.error } } }`,
  ))
  createHarness = module.createHarness
  fixture = await import(/* @vite-ignore */ `data:text/javascript;base64,${Buffer.from(clientCode(`
    import { flushSync } from ${JSON.stringify(clientEntry)};
    export { flushSync };
    export function root(callback) { return $effect.root(callback) }
    export function runtime() { const value = $state({
      state: { route: { kind: "project-members", workspaceID: "workspace", projectID: "first" } },
      auth: { state: { status: "authenticated", claims: { principal: { ref: { id: "alice" } } } } },
      access: { state: { workspaceStatus: "ready", workspaces: [{ id: "workspace", name: "Workspace" }] } },
    }); return value }
  `)).toString("base64")}`)
}, 30000)

function setup() {
  const runtime = fixture.runtime()
  let refresh!: (event: { signal: AbortSignal }) => Promise<void>
  const unsubscribe = vi.fn()
  Object.assign(runtime, {
    activity: {
      subscribe: vi.fn((_selectors, callback) => { refresh = callback; return unsubscribe }),
      poll: vi.fn(async () => {}),
    },
    requireLogin: vi.fn(),
    refresh: vi.fn(async () => true),
    navigate: vi.fn(),
  })
  let page: any
  const stop = fixture.root(() => { page = createHarness(runtime) })
  fixture.flushSync()
  return { runtime, page, stop, unsubscribe, refresh: () => refresh({ signal: new AbortController().signal }) }
}

describe("project members route lifecycle", () => {
  it("loads a deep-linked project and preserves its page during a failed metadata refresh", async () => {
    const project = { id: "first", name: "Launch" }
    const request = vi.fn(async () => Response.json(project))
    vi.stubGlobal("fetch", request)
    const { page, stop, refresh, unsubscribe } = setup()
    try {
      await vi.waitFor(() => expect(page.project).toEqual(project))
      request.mockResolvedValue(new Response("unavailable", { status: 503 }))
      await expect(refresh()).rejects.toThrow("could not be refreshed")
      expect(page.project).toEqual(project)
      expect(page.error).toBeTruthy()
    } finally { stop() }
    expect(unsubscribe).toHaveBeenCalledOnce()
  })

  it("aborts old project work and rejects late decoding after navigating to another Members page", async () => {
    let decode!: (project: unknown) => void
    let oldSignal: AbortSignal | undefined
    vi.stubGlobal("fetch", vi.fn(async (path: string, init: RequestInit) => {
      if (path.endsWith("/first")) {
        oldSignal = init.signal as AbortSignal
        return { ok: true, status: 200, json: () => new Promise((resolve) => { decode = resolve }) } as Response
      }
      return Response.json({ id: "second", name: "Second project" })
    }))
    const { page, runtime, stop } = setup()
    try {
      await vi.waitFor(() => expect(decode).toBeTypeOf("function"))
      runtime.state.route = { kind: "project-members", workspaceID: "workspace", projectID: "second" }
      fixture.flushSync()
      await vi.waitFor(() => expect(page.project?.id).toBe("second"))
      decode({ id: "first", name: "Old project" })
      await Promise.resolve()
      fixture.flushSync()
      expect(oldSignal?.aborted).toBe(true)
      expect(page.project.id).toBe("second")
      expect(runtime.navigate).not.toHaveBeenCalled()
    } finally { stop() }
  })

  it("redirects a missing or inaccessible project to the project directory", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("not found", { status: 404 })))
    const { runtime, page, stop } = setup()
    try {
      await vi.waitFor(() => expect(runtime.navigate).toHaveBeenCalledWith("/app/wsp/workspace/prj", true))
      expect(page.project).toBeNull()
    } finally { stop() }
  })
})
