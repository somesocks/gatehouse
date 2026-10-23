import { afterEach, expect, it, vi } from "vitest"
import { importClientModule } from "../testing/client-module"

afterEach(() => vi.unstubAllGlobals())

it("keeps sign-in links and authentication redirects aligned with the complete current URL", async () => {
  let location = new URL("https://gatehouse.test/app/wsp/workspace/prj?name=a%26b#selected")
  const change = (_state: unknown, _title: string, path: string) => {
    location = new URL(path, location)
  }
  const pushState = vi.fn(change)
  const replaceState = vi.fn(change)
  vi.stubGlobal("window", {
    get location() { return location },
    history: { pushState, replaceState },
  })
  const { createApplicationRuntime } = await importClientModule(
    new URL("./runtime.svelte.ts", import.meta.url),
  )
  const runtime = createApplicationRuntime()

  expect(new URL(runtime.loginHref, location).searchParams.get("next"))
    .toBe("/app/wsp/workspace/prj?name=a%26b#selected")

  runtime.navigate("/app/wsp/workspace/ses/chat/notes?extra=1#revision")
  expect(new URL(runtime.loginHref, location).searchParams.get("next"))
    .toBe("/app/wsp/workspace/ses/chat/notes?extra=1#revision")

  // Query/fragment changes can leave the parsed route unchanged.
  runtime.navigate("/app/wsp/workspace/ses/chat/notes?extra=2#latest")
  const href = runtime.loginHref
  expect(new URL(href, location).searchParams.get("next"))
    .toBe("/app/wsp/workspace/ses/chat/notes?extra=2#latest")

  runtime.requireLogin()
  expect(replaceState).toHaveBeenLastCalledWith(null, "", href)
  expect(runtime.state.route.kind).toBe("login")
})
