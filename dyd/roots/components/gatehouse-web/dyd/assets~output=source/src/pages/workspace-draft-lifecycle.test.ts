import { readFileSync } from "node:fs"
import { createSourceFile, ScriptTarget, isImportDeclaration } from "typescript"
import { afterEach, describe, expect, it, vi } from "vitest"
import {
  clientCode,
  clientEntry,
  clientModuleURL,
} from "../testing/client-module"

afterEach(() => vi.unstubAllGlobals())

// Exercise the actual route's script and activation effect with the Svelte client
// runtime. Presentation imports are omitted because this test needs no DOM.
async function routeHarness(
  path: string,
  draft: string,
  status = "detailStatus",
  failure = "detailLoad.state.error",
  extra = "",
) {
  const url = new URL(path, import.meta.url)
  const source = readFileSync(url, "utf8").match(
    /<script lang="ts">([\s\S]*?)<\/script>/,
  )![1]
  const ast = createSourceFile(path, source, ScriptTarget.Latest, true)
  const imports = ast.statements.filter(isImportDeclaration)
  const kept = imports
    .filter(
      (statement) =>
        !/ui|runtime\.svelte/.test(statement.moduleSpecifier.getText(ast)),
    )
    .map((statement) => statement.getText(ast))
  let body = source
  for (const statement of [...imports].reverse())
    body = body.slice(0, statement.getStart(ast)) + body.slice(statement.end)
  body = body.replace(/const runtime = useRuntime\(\)/, "")
  // Lower the resource owners' reactive component props for this DOM-free module.
  body = body.replace(
    /let \{ embedded = false, route, suspended = false(?:, dialog)? \}:[^\n]+? = \$props\(\)/,
    "const embedded = $derived(props.embedded ?? false); const route = $derived(props.route); const suspended = $derived(props.suspended ?? false); const dialog = $derived(props.dialog)",
  )
  body = body.replace(/\$props\(\)/g, "props")
  const module = await import(
    /* @vite-ignore */ clientModuleURL(
      url,
      `${kept.join("\n")}\nexport function createHarness(runtime, props = {}) { ${body}\nreturn { get draft() { return ${draft} }, setDraft(value) { ${draft} = value }, get detailStatus() { return ${status} }, get currentRoute() { return currentRoute }, get active() { return active }, get error() { return ${failure} }, ${extra} } }`,
    )
  )
  const fixture = await import(
    /* @vite-ignore */ `data:text/javascript;base64,${Buffer.from(clientCode(`import { flushSync } from ${JSON.stringify(clientEntry)}; export { flushSync }; export function root(callback) { return $effect.root(callback) } export function createRuntime(route) { const runtime = $state({ state: { route }, auth: { state: { status: "authenticated", claims: { principal: { ref: { id: "principal" } } } } }, access: { state: { workspaceStatus: "ready", workspaces: [{ id: "workspace", name: "Workspace" }] } } }); return runtime }`)).toString("base64")}`
  )
  const svelte = fixture
  return { module, fixture, svelte }
}

const routes = [
  {
    path: "./project-notes/ProjectNotesRoute.svelte",
    draft: "title",
    route: {
      kind: "project-note-edit",
      workspaceID: "workspace",
      projectID: "owner",
      noteID: "item",
    },
  },
  {
    path: "./session-notes/SessionNotesRoute.svelte",
    draft: "title",
    route: {
      kind: "session-note-edit",
      workspaceID: "workspace",
      sessionID: "owner",
      noteID: "item",
    },
  },
  {
    path: "./tasks/TasksRoute.svelte",
    draft: "title",
    route: {
      kind: "project-task-edit",
      workspaceID: "workspace",
      projectID: "owner",
      taskID: "item",
    },
  },
]
const item = {
  id: "item",
  title: "Server title",
  description: "Server description",
  body: "Body",
  sensitive: false,
  status: "draft",
  revision: 1,
}

describe("workspace edit route lifecycle", () => {
  it.each(routes)(
    "$path retains drafts and content across access changes and failed reconciliation",
    async ({ path, draft, route }) => {
      const { module, fixture, svelte } = await routeHarness(path, draft)
      let reconcile!: (event: { signal: AbortSignal }) => Promise<void>
      const runtime = Object.assign(fixture.createRuntime(route), {
        activity: {
          subscribe: vi.fn((_topics, callback) => {
            reconcile = callback
            return () => {}
          }),
          poll: vi.fn(async () => {}),
        },
        navigate: vi.fn(),
        requireLogin: vi.fn(),
        refresh: vi.fn(async () => true),
      })
      const request = vi.fn(async (url: string) =>
        Response.json(
          url.endsWith("/item") ? item : { id: "owner", name: "Owner" },
        ),
      )
      vi.stubGlobal("fetch", request)
      let controller: ReturnType<typeof module.createHarness>
      const dispose = fixture.root(() => {
        controller = module.createHarness(runtime)
      })
      try {
        svelte.flushSync()
        await vi.waitFor(() => expect(controller!.detailStatus).toBe("ready"))
        controller!.setDraft("Unsaved local edit")
        const calls = request.mock.calls.length
        runtime.access.state.workspaces = [
          { id: "workspace", name: "Renamed workspace" },
        ]
        svelte.flushSync()
        expect(request).toHaveBeenCalledTimes(calls)
        expect(controller!.draft).toBe("Unsaved local edit")
        let finish!: (response: Response) => void
        request.mockImplementation(async (url) =>
          url.endsWith("/item")
            ? await new Promise<Response>((resolve) => {
                finish = resolve
              })
            : Response.json({ id: "owner", name: "Owner" }),
        )
        const reloading = reconcile({ signal: new AbortController().signal })
        expect(controller!.detailStatus).toBe("ready")
        finish(new Response(null, { status: 503 }))
        await expect(reloading).rejects.toThrow()
        expect(controller!.active).toEqual(item)
        expect(controller!.detailStatus).toBe("ready")
        expect(controller!.draft).toBe("Unsaved local edit")
        expect(controller!.error).not.toBe("")
      } finally {
        dispose()
      }
    },
    60000,
  )
})

describe("closing resource route ownership", () => {
  it.each([
    { path: "./session-files/SessionFilesRoute.svelte", draft: "page.state.search", status: "filesStatus", failure: "filesLoad.state.error", route: { kind: "session-files", workspaceID: "workspace", sessionID: "owner" } },
    { path: "./session-notes/SessionNotesRoute.svelte", draft: "title", status: "detailStatus", failure: "detailLoad.state.error", route: { kind: "session-note", workspaceID: "workspace", sessionID: "owner", noteID: "item" } },
    { path: "./tasks/TasksRoute.svelte", draft: "title", status: "detailStatus", failure: "detailLoad.state.error", route: { kind: "session-task", workspaceID: "workspace", sessionID: "owner", taskID: "item" } },
    { path: "./session-secrets/SessionSecretsRoute.svelte", draft: "secretBreadcrumb", status: "sessionStatus", failure: "resource.state.error", route: { kind: "session-secret", workspaceID: "workspace", sessionID: "owner", secretID: "item" } },
  ])("$path suspends requests while retaining its displayed route and snapshot during dismissal", async ({ path, draft, status, failure, route }) => {
    const { module, fixture, svelte } = await routeHarness(path, draft, status, failure)
    const subscriptions: ReturnType<typeof vi.fn>[] = []
    const runtime = Object.assign(fixture.createRuntime(route), {
      activity: { subscribe: vi.fn(() => {
        const unsubscribe = vi.fn()
        subscriptions.push(unsubscribe)
        return unsubscribe
      }), poll: vi.fn(async () => {}) },
      navigate: vi.fn(), requireLogin: vi.fn(), refresh: vi.fn(async () => true),
    })
    const request = vi.fn(async (url: string, _options: RequestInit) => Response.json(
      url.includes("/files") ? [] : url.endsWith("/item") ? item : { id: "owner", name: "Chat" },
    ))
    vi.stubGlobal("fetch", request)
    const props = fixture.createRuntime(route).state
    props.embedded = true
    props.suspended = false
    let controller!: ReturnType<typeof module.createHarness>
    const dispose = fixture.root(() => {
      controller = module.createHarness(runtime, props)
    })
    try {
      svelte.flushSync()
      await vi.waitFor(() => expect(controller.detailStatus).toBe("ready"))
      if (route.kind === "session-files") {
        const filesRequest = request.mock.calls.find(([url]) => url.includes("/files"))
        expect(new URL(filesRequest![0], "http://localhost").searchParams.get("author")).toBe("all")
      }
      controller.setDraft("Retained draft")
      const calls = request.mock.calls.length
      expect(runtime.activity.subscribe).toHaveBeenCalled()
      runtime.state.route = { kind: "session-chat", workspaceID: "workspace", sessionID: "owner" }
      props.suspended = true
      svelte.flushSync()
      expect(controller.currentRoute).toEqual(route)
      expect(controller.detailStatus).toBe("ready")
      expect(controller.draft).toBe("Retained draft")
      expect(request).toHaveBeenCalledTimes(calls)
      for (const unsubscribe of subscriptions) expect(unsubscribe).toHaveBeenCalledOnce()
      expect(request.mock.calls.every(([, options]) => options.signal?.aborted)).toBe(true)
      expect(runtime.navigate).not.toHaveBeenCalled()
    } finally {
      dispose()
    }
  }, 60000)
})

describe("native record loading and pagination", () => {
  const route = {
    kind: "project-record-new",
    workspaceID: "workspace",
    projectID: "owner",
    schemaID: "type",
  }
  const schema = {
    id: "type",
    name: "type",
    label: "Type",
    description: "",
    created_at: "2026-01-01T00:00:00.000Z",
  }
  const attribute = {
    id: "field",
    name: "name",
    label: "Name",
    type: "text",
    cardinality: "one",
    uniqueness: "none",
    display: "primary",
    display_order: 0,
  }
  const extra =
    "get schema() { return schema }, get attributes() { return attributes }, get references() { return incomingReferences }, get referenceCursor() { return incomingPage.state.cursor }, get referenceNext() { return incomingPage.state.next }, nextReferences() { return incomingPage.next(loadMoreIncomingReferences) }, nextValues() { return valuesPage.next(reloadValues) }"
  const fixtureRuntime = (fixture, selected) =>
    Object.assign(fixture.createRuntime(selected), {
      activity: { subscribe: () => () => {}, poll: async () => {} },
      navigate: vi.fn(),
      requireLogin: vi.fn(),
      refresh: vi.fn(async () => true),
    })

  it("settles create mode without requesting an existing record", async () => {
    const { module, fixture, svelte } = await routeHarness(
      "./project-records/ProjectRecordsRoute.svelte",
      "drafts",
      "recordsStatus",
      "schemaLoad.state.error",
      extra,
    )
    const request = vi.fn(async (url: string) =>
      Response.json(
        url.includes("/attributes")
          ? [attribute]
          : url.endsWith("/type")
            ? schema
            : { id: "owner", name: "Owner" },
      ),
    )
    vi.stubGlobal("fetch", request)
    let controller
    const dispose = fixture.root(() => {
      controller = module.createHarness(fixtureRuntime(fixture, route))
    })
    try {
      svelte.flushSync()
      await vi.waitFor(() => expect(controller.attributes.length).toBe(1))
      expect(controller.detailStatus).toBe("ready")
      expect(controller.schema).toEqual(schema)
      expect(controller.draft).toEqual([])
      expect(
        request.mock.calls.every(([url]) => !url.includes("/records")),
      ).toBe(true)
    } finally {
      dispose()
    }
  }, 60000)

  it("keeps value drafts on Next and requests the pending reverse-reference cursor", async () => {
    const { module, fixture, svelte } = await routeHarness(
      "./project-records/ProjectRecordsRoute.svelte",
      "drafts",
      "recordsStatus",
      "recordLoad.state.error || referencesLoad.state.error",
      extra,
    )
    const record = {
      id: "item",
      schema: { id: "type" },
      created_at: "2026-01-01T00:00:00.000Z",
    }
    const group = {
      source_schema: { id: "source", label: "Source" },
      source_attribute: { id: "source-field", label: "Reference" },
      references: [{ id: "other" }],
    }
    const request = vi.fn(async (url: string) => {
      if (url.includes("/attributes")) return Response.json([attribute])
      if (url.includes("/references"))
        return Response.json({
          groups: [group],
          next_cursor: "references-next",
        })
      if (url.includes("/values")) {
        const second =
          new URL(url, "http://localhost").searchParams.get("cursor") ===
          "values-next"
        return Response.json({
          values: [
            {
              id: second ? "second-value" : "first-value",
              attribute: "field",
              value: "Server value",
              sensitive: false,
            },
          ],
          ...(second ? {} : { next_cursor: "values-next" }),
        })
      }
      if (url.includes("/records?")) return Response.json({ records: [record] })
      return Response.json(
        url.endsWith("/item")
          ? record
          : url.endsWith("/type")
            ? schema
            : { id: "owner", name: "Owner" },
      )
    })
    vi.stubGlobal("fetch", request)
    const runtime = fixtureRuntime(fixture, {
      ...route,
      kind: "project-record-edit",
      recordID: "item",
    })
    let controller
    const dispose = fixture.root(() => {
      controller = module.createHarness(runtime)
    })
    try {
      svelte.flushSync()
      await vi.waitFor(() => expect(controller.draft.length).toBe(1))
      controller.draft[0].value = "Unsaved local value"
      await controller.nextValues()
      expect(controller.draft.map((value) => value.id)).toEqual([
        "first-value",
        "second-value",
      ])
      expect(controller.draft[0].value).toBe("Unsaved local value")
      runtime.state.route = {
        ...route,
        kind: "project-record",
        recordID: "item",
      }
      svelte.flushSync()
      await vi.waitFor(() => expect(controller.references.length).toBe(1))
      const original = request.getMockImplementation()!
      request.mockImplementation(async (url) =>
        url.includes("/references?")
          ? new Response(null, { status: 503 })
          : await original(url),
      )
      expect(await controller.nextReferences()).toBe(false)
      expect(controller.referenceCursor).toBe("")
      expect(controller.referenceNext).toBe("references-next")
      expect(controller.references).toEqual([group])
      expect(
        request.mock.calls.some(([url]) =>
          url.endsWith("/references?cursor=references-next"),
        ),
      ).toBe(true)
    } finally {
      dispose()
    }
  }, 60000)
})
