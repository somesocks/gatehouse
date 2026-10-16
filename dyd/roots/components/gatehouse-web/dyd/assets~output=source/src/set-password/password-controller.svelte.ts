import { createLoadState } from "../utils/load-state.svelte"

export function createPasswordController(fragment: string) {
  const parameters = new URLSearchParams(fragment.replace(/^#/, ""))
  let capability = parameters.get("capability") ?? ""
  const requestState = createLoadState()
  const state = $state({
    status: "loading" as
      | "loading"
      | "ready"
      | "invalid"
      | "unavailable"
      | "success",
    username: "",
    expiresAt: "",
    password: "",
    confirmation: "",
    error: "",
    get pending() {
      return requestState.state.pending
    },
  })
  const invalidLink =
    "This link is invalid or no longer available. Ask an administrator for a new link."

  function invalidate(message: string) {
    capability = ""
    state.status = "invalid"
    state.error = message
  }
  async function send(
    method: "GET" | "POST",
    signal: AbortSignal,
    password?: string,
  ) {
    return await fetch("/api/v1/auth/set-password", {
      method,
      credentials: "omit",
      cache: "no-store",
      headers: {
        Authorization: `Bearer ${capability}`,
        ...(method === "POST" ? { "Content-Type": "application/json" } : {}),
      },
      ...(method === "POST" ? { body: JSON.stringify({ password }) } : {}),
      signal,
    })
  }
  async function load() {
    if (state.pending) return
    if (!capability || parameters.getAll("capability").length !== 1) {
      invalidate(invalidLink)
      return
    }
    const request = requestState.begin()
    state.status = "loading"
    state.error = ""
    try {
      const response = await send("GET", request.signal)
      if (!request.current()) return
      if ([401, 409, 410].includes(response.status)) {
        const message = await response.text()
        if (request.current()) invalidate(message.trim() || invalidLink)
      } else if (!response.ok) throw new Error("unavailable")
      else {
        const details = (await response.json()) as {
          username: string
          expires_at: string
        }
        if (!request.current()) return
        state.username = details.username
        state.expiresAt = details.expires_at
        state.status = "ready"
      }
    } catch {
      if (request.current()) {
        state.status = "unavailable"
        state.error = "The link could not be checked. Try again."
      }
    } finally {
      request.finish()
    }
  }
  async function save() {
    if (state.pending || state.status !== "ready") return
    state.error = ""
    if (!state.password) {
      state.error = "A password is required."
      return
    }
    if (state.password !== state.confirmation) {
      state.error = "Passwords do not match."
      return
    }
    const request = requestState.begin()
    try {
      const response = await send("POST", request.signal, state.password)
      if (!request.current()) return
      if ([401, 409, 410].includes(response.status)) {
        const message = await response.text()
        if (request.current()) invalidate(message.trim() || invalidLink)
      } else if (response.status === 400) {
        const message = await response.text()
        if (request.current())
          state.error = message.trim() || "Check the new password."
      } else if (!response.ok) throw new Error("unavailable")
      else {
        capability = ""
        state.status = "success"
      }
    } catch {
      if (request.current())
        state.error = "Password could not be saved. Try again."
    } finally {
      if (request.current()) state.password = state.confirmation = ""
      request.finish()
    }
  }
  function stop() {
    requestState.stop()
    capability = ""
    state.password = state.confirmation = ""
  }
  return { state, load, save, stop }
}
