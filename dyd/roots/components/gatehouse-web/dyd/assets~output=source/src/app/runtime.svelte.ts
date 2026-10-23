import { getContext, setContext } from "svelte"
import { createAccess } from "./access.svelte"
import { createActivityClient } from "./activity"
import { createAuth } from "./auth.svelte"
import { createRouter, type Navigate } from "./router"
import { parseRoute, type Route } from "../route"
import { loginPath } from "./login-path"

const runtimeContext = Symbol("gatehouse-runtime")

export type ApplicationRuntime = ReturnType<typeof createApplicationRuntime>

export function createApplicationRuntime() {
  const state = $state<{ route: Route }>({
    route: parseRoute(new URL(window.location.href)),
  })
  let started = false
  let stopRouter: (() => void) | undefined
  let accessPrincipalID: string | undefined
  let initializing: Promise<boolean> | null = null
  let handlingAuthenticationLoss = false
  let generation = 0

  const auth = createAuth()
  const activity = createActivityClient({ onAuthenticationLost: requireLogin })
  const access = createAccess({ activity, onAuthenticationLost: requireLogin })
  const router = createRouter((route) => (state.route = route))
  function getLoginHref(): string {
    // Track navigation while retaining the original query and fragment.
    state.route
    return loginPath(
      window.location.pathname + window.location.search + window.location.hash,
    )
  }

  function navigate(path: string, replace = false): void {
    router.navigate(path, replace)
  }

  function requireLogin(): void {
    if (handlingAuthenticationLoss) return
    handlingAuthenticationLoss = true
    generation++
    initializing = null
    accessPrincipalID = undefined
    access.clear()
    activity.dispose()
    auth.clear()
    if (state.route.kind !== "login") {
      navigate(getLoginHref(), true)
    }
  }

  async function refresh(): Promise<boolean> {
    if (initializing !== null) return await initializing
    const value = generation
    const pending = (async () => {
      const status = await auth.check()
      if (value !== generation || auth.state.reloadError) return false
      if (status !== "authenticated") {
        if (status === "anonymous") requireLogin()
        return false
      }
      handlingAuthenticationLoss = false
      const principalID = auth.state.claims?.principal.ref.id
      if (principalID === undefined) {
        requireLogin()
        return false
      }
      if (accessPrincipalID !== principalID) {
        access.start(principalID)
        accessPrincipalID = principalID
      }
      const loaded = await access.refresh()
      if (value === generation) router.resolve()
      return loaded && value === generation
    })()
    initializing = pending
    try {
      return await pending
    } finally {
      if (initializing === pending) initializing = null
    }
  }

  function start(): void {
    if (started) return
    started = true
    stopRouter = router.start()
    router.resolve()
    void refresh()
  }

  function stop(): void {
    if (!started) return
    started = false
    generation++
    initializing = null
    stopRouter?.()
    stopRouter = undefined
    access.clear()
    auth.stop()
    activity.dispose()
  }

  return {
    state,
    auth,
    access,
    activity,
    router,
    navigate: navigate as Navigate,
    get loginHref() {
      return getLoginHref()
    },
    requireLogin,
    refresh,
    start,
    stop,
  }
}

export function provideRuntime(
  runtime: ApplicationRuntime,
): ApplicationRuntime {
  setContext(runtimeContext, runtime)
  return runtime
}

export function useRuntime(): ApplicationRuntime {
  const runtime = getContext<ApplicationRuntime>(runtimeContext)
  if (runtime === undefined)
    throw new Error("Gatehouse application runtime is missing")
  return runtime
}
