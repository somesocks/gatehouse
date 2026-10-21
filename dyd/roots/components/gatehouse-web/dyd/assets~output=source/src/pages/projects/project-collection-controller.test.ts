import { afterEach, beforeAll, expect, it, vi } from "vitest"
import { importClientModule } from "../../testing/client-module"
import type { ActivityRefresh, ActivitySelector } from "../../app/activity"

afterEach(() => vi.unstubAllGlobals())
let client: Awaited<ReturnType<typeof importClientModule>>
beforeAll(async () => {
  client = await importClientModule(new URL("./project-collection-controller.svelte.ts", import.meta.url))
}, 30000)

it("reconciles grants and memberships without replacing an applied filter or draft", async () => {
  let refresh!: ActivityRefresh
  const unsubscribe = vi.fn()
  const activity = {
    subscribe: vi.fn((_selectors: Iterable<ActivitySelector>, callback: ActivityRefresh) => {
      refresh = callback
      return unsubscribe
    }),
    poll: vi.fn(async () => {}),
    dispose: vi.fn(),
  }
  const project = { id: "prj_a", name: "Launch", created_at: "2026-01-01T00:00:00Z" }
  const request = vi.fn(async () => Response.json({ projects: [project] }))
  vi.stubGlobal("fetch", request)
  const controller = client.createProjectCollectionController({ activity, onAuthenticationLost: vi.fn() })
  const parent = new AbortController()
  controller.start("workspace", "Launch", parent.signal)
  await vi.waitFor(() => expect(controller.state.projects).toEqual([project]))
  controller.state.search = "unsent draft"
  request.mockResolvedValue(Response.json({ projects: [] }))
  await refresh({ names: new Set(["project-directory-access"]), signal: new AbortController().signal })
  expect(controller.state.projects).toEqual([])
  expect(controller.state.search).toBe("unsent draft")
  expect(request.mock.calls.at(-1)?.[0]).toContain("name=Launch")
  expect(activity.subscribe.mock.calls[0][0]).toEqual([
    expect.objectContaining({ topic: "workspace", events: expect.arrayContaining(["project_grant.*", "group_member.*"]) }),
  ])
  parent.abort()
  controller.stop()
  expect(unsubscribe).toHaveBeenCalledOnce()
})
