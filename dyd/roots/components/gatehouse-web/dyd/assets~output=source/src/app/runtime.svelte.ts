import { getContext, setContext } from "svelte"
import { createAccess } from "./access.svelte"
import { createActivityClient } from "./activity"
import { createAuth } from "./auth.svelte"
import { createRouter, type Navigate } from "./router"
import { parseRoute, type Route } from "../route"

const runtimeContext = Symbol("gatehouse-runtime")

export type ApplicationRuntime = ReturnType<typeof createApplicationRuntime>

export function createApplicationRuntime() {
  const state = $state<{ route: Route }>({
    route: parseRoute(new URL(window.location.href)),
  })
  let started = false
  let stopRouter: (() => void) | undefined
  let accessPrincipalID: string | undefined
  let initializing: Promise<void> | null = null
  let handlingAuthenticationLoss = false

  const auth = createAuth()
  const activity = createActivityClient({ onAuthenticationLost: requireLogin })
  const access = createAccess({ activity, onAuthenticationLost: requireLogin })
  const router = createRouter((route) => (state.route = route))

  function navigate(path: string, replace = false): void {
    router.navigate(path, replace)
  }

  function requireLogin(): void {
    if (handlingAuthenticationLoss) return
    handlingAuthenticationLoss = true
    accessPrincipalID = undefined
    access.clear()
    activity.dispose()
    auth.clear()
    if (state.route.kind !== "login") {
      const requested =
        window.location.pathname + window.location.search + window.location.hash
      navigate(`/app/login?next=${encodeURIComponent(requested)}`, true)
    }
  }

  async function refresh(): Promise<void> {
    if (initializing !== null) return await initializing
    const pending = (async () => {
      const status = await auth.check()
      if (status !== "authenticated") {
        if (status === "anonymous") requireLogin()
        return
      }
      handlingAuthenticationLoss = false
      const principalID = auth.state.claims?.principal.ref.id
      if (principalID === undefined) {
        requireLogin()
        return
      }
      if (accessPrincipalID !== principalID) {
        access.start(principalID)
        accessPrincipalID = principalID
      }
      await access.refresh()
      router.resolve()
    })()
    initializing = pending
    try {
      await pending
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
    stopRouter?.()
    stopRouter = undefined
    access.stop()
    activity.dispose()
  }

  return {
    state,
    auth,
    access,
    activity,
    router,
    navigate: navigate as Navigate,
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
