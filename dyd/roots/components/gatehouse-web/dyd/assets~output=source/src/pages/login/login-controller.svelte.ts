import type { Auth } from "../../app/auth.svelte"

export function createLoginController(auth: Auth, onAuthenticated: () => void) {
  const state = $state({ identity: "", password: "", submitting: false, error: "" })

  async function submit(): Promise<void> {
    state.error = ""
    state.submitting = true
    try {
      const outcome = await auth.signIn(state.identity, state.password)
      if (outcome === "authenticated") {
        state.password = ""
        onAuthenticated()
      } else if (outcome === "invalid" || outcome === "anonymous") {
        state.error = "The username or password is incorrect."
      } else {
        state.error = "Gatehouse could not be reached. Try again."
      }
    } finally {
      state.submitting = false
    }
  }

  return { state, submit }
}
