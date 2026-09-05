import { checkAuthentication, type Claims } from "./auth"

export type AuthenticationStatus = "checking" | "anonymous" | "authenticated" | "unavailable"

type AuthenticationTransport = {
  check: () => Promise<Claims | null>
}

export function createAuth(transport: AuthenticationTransport = { check: checkAuthentication }) {
  const state = $state<{ status: AuthenticationStatus; claims: Claims | null }>({ status: "checking", claims: null })
  let checkPromise: Promise<AuthenticationStatus> | null = null
  let generation = 0

  async function check(): Promise<AuthenticationStatus> {
    if (checkPromise !== null) {
      return await checkPromise
    }
    const currentGeneration = ++generation
    state.status = "checking"
    const pending = (async () => {
      try {
        const claims = await transport.check()
        if (currentGeneration !== generation) {
          return state.status
        }
        state.claims = claims
        state.status = claims === null ? "anonymous" : "authenticated"
      } catch {
        if (currentGeneration === generation) {
          state.claims = null
          state.status = "unavailable"
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
      }
    }
  }

  function clear(): void {
    generation += 1
    state.claims = null
    state.status = "anonymous"
  }

  return { state, check, clear }
}
