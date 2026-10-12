import {
  checkAuthentication,
  signIn as submitCredentials,
  type Claims,
} from "./auth"
import { clearAccessToken } from "./api"

export type AuthenticationStatus =
  | "checking"
  | "anonymous"
  | "authenticated"
  | "unavailable"

type AuthenticationTransport = {
  check: (signal?: AbortSignal) => Promise<Claims | null>
  signIn: (
    identity: string,
    password: string,
    signal?: AbortSignal,
  ) => Promise<boolean>
}

export type Auth = ReturnType<typeof createAuth>

export function createAuth(
  transport: AuthenticationTransport = {
    check: checkAuthentication,
    signIn: submitCredentials,
  },
) {
  const state = $state<{
    status: AuthenticationStatus
    claims: Claims | null
    reloading: boolean
    reloadError: string
  }>({ status: "checking", claims: null, reloading: false, reloadError: "" })
  let checkPromise: Promise<AuthenticationStatus> | null = null
  let generation = 0
  let controller: AbortController | undefined

  async function check(): Promise<AuthenticationStatus> {
    if (checkPromise !== null) {
      return await checkPromise
    }
    const currentGeneration = ++generation
    controller?.abort()
    controller = new AbortController()
    const signal = controller.signal
    const loaded =
      state.status === "authenticated" || state.status === "anonymous"
    if (!loaded) state.status = "checking"
    state.reloading = loaded
    state.reloadError = ""
    const pending = (async () => {
      try {
        const claims = await transport.check(signal)
        if (currentGeneration !== generation) {
          return state.status
        }
        state.claims = claims
        state.status = claims === null ? "anonymous" : "authenticated"
      } catch {
        if (currentGeneration === generation) {
          if (!loaded) {
            state.claims = null
            state.status = "unavailable"
          }
          state.reloadError = "Authentication could not be refreshed."
        }
      }
      return state.status
    })()
    checkPromise = pending
    try {
      return await pending
    } finally {
      if (checkPromise === pending) {
        checkPromise = null
        state.reloading = false
      }
    }
  }

  function stop(): void {
    generation += 1
    controller?.abort()
    checkPromise = null
    state.reloading = false
  }

  function clear(): void {
    stop()
    clearAccessToken()
    state.claims = null
    state.status = "anonymous"
    state.reloading = false
    state.reloadError = ""
    checkPromise = null
  }

  async function signIn(
    identity: string,
    password: string,
  ): Promise<AuthenticationStatus | "invalid"> {
    const currentGeneration = ++generation
    controller?.abort()
    controller = new AbortController()
    const signal = controller.signal
    checkPromise = null
    try {
      const valid = await transport.signIn(identity, password, signal)
      if (currentGeneration !== generation || signal.aborted)
        return state.status
      if (!valid) {
        return "invalid"
      }
      return await check()
    } catch {
      if (currentGeneration !== generation || signal.aborted)
        return state.status
      return "unavailable"
    }
  }

  return { state, check, clear, signIn, stop }
}
