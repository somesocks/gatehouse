import {
  systemAdministration,
  type SystemPrincipal,
  type SystemPrincipalIdentity,
} from "../../../app/system"
import { createLoadState } from "../../../utils/load-state.svelte"

export function createPrincipalLoginsController({
  onAuthenticationLost,
  onSystemAccessChange,
  onChanged,
  currentIdentityID,
  onReload,
}: {
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
  onChanged: (principal: SystemPrincipal) => void
  currentIdentityID: () => string | undefined
  onReload: () => Promise<void>
}) {
  const mutation = createLoadState()
  let owner = ""
  let disposed = false
  let generation = 0
  const state = $state({
    mode: null as "add" | "edit" | "password" | null,
    identity: null as SystemPrincipalIdentity | null,
    username: "",
    password: "",
    confirmation: "",
    enabled: true,
    error: "",
    notice: "",
    conflict: false,
    passwordLink: null as {
      url: string
      expires_at: string
      username: string
    } | null,
    get saving() {
      return mutation.state.pending
    },
  })
  function cancel() {
    if (state.saving) return
    state.mode = null
    state.identity = null
    state.username = state.password = state.confirmation = ""
    state.error = ""
    state.conflict = false
    state.passwordLink = null
  }
  function reset(principalID: string) {
    generation++
    mutation.stop()
    owner = principalID
    disposed = false
    cancel()
    state.notice = ""
  }
  function begin(
    mode: "add" | "edit" | "password",
    identity?: SystemPrincipalIdentity,
  ) {
    if (disposed || state.saving) return
    cancel()
    state.mode = mode
    state.identity = identity ? { ...identity } : null
    state.username = identity?.key.startsWith("gatehouse:")
      ? identity.key.slice("gatehouse:".length)
      : ""
    state.enabled = identity?.enabled ?? true
    state.notice = ""
  }
  async function mutate(
    method: "POST" | "PATCH" | "DELETE",
    identity: SystemPrincipalIdentity | null,
    body: Record<string, unknown>,
    signsOut = false,
  ): Promise<boolean> {
    if (disposed || state.saving) return false
    const request = mutation.begin()
    state.error = state.notice = ""
    try {
      const path = `principals/${encodeURIComponent(owner)}/identities${identity ? `/${encodeURIComponent(identity.id)}` : ""}`
      const response = await systemAdministration(
        path,
        method,
        body,
        request.signal,
      )
      if (!request.current()) return false
      if (response.status === 401) onAuthenticationLost()
      else if (response.status === 403) onSystemAccessChange("denied")
      else if (response.status === 409) {
        const message = await response.text()
        if (!request.current()) return false
        state.conflict = message.includes("revision conflict")
        state.error = state.conflict
          ? "This login changed elsewhere. Cancel and reopen it to use the latest version."
          : "That username is already in use."
        if (state.conflict) await onReload()
      } else if (response.status === 400) {
        const message = await response.text()
        if (request.current())
          state.error = message.trim() || "Check the login details."
      } else if (response.status === 404) {
        state.error = "This principal or login no longer exists."
        await onReload()
      } else if (!response.ok) state.error = "Login could not be saved."
      else {
        const principal = (await response.json()) as SystemPrincipal
        if (!request.current()) return false
        onChanged(principal)
        if (signsOut && identity?.id === currentIdentityID())
          onAuthenticationLost()
        return true
      }
    } catch {
      if (request.current()) state.error = "Login could not be saved."
    } finally {
      request.finish()
    }
    return false
  }
  async function save(): Promise<boolean> {
    if (disposed || state.saving || !state.mode || state.conflict) return false
    const mode = state.mode
    const current = generation
    if (mode !== "edit" && state.password === "") {
      state.error = "A password is required."
      return false
    }
    if (mode !== "edit" && state.password !== state.confirmation) {
      state.error = "Passwords do not match."
      return false
    }
    const body: Record<string, unknown> =
      mode === "password"
        ? { password: state.password }
        : {
            username: state.username,
            enabled: state.enabled,
            ...(mode === "add" ? { password: state.password } : {}),
          }
    if (state.identity) body.expected_revision = state.identity.revision
    const saved = await mutate(
      mode === "add" ? "POST" : "PATCH",
      state.identity,
      body,
      mode === "edit" && !state.enabled,
    )
    if (disposed || current !== generation) return false
    // Never retain a submitted password in the form, including after errors.
    state.password = state.confirmation = ""
    if (saved) {
      cancel()
      state.notice =
        mode === "password"
          ? "Password updated. Existing sessions remain active."
          : "Login saved."
    }
    return saved
  }
  async function setEnabled(identity: SystemPrincipalIdentity): Promise<void> {
    if (state.saving || disposed) return
    if (identity.enabled && !window.confirm(`Disable ${identity.key}?`)) return
    const current = generation
    const saved = await mutate(
      "PATCH",
      identity,
      { enabled: !identity.enabled, expected_revision: identity.revision },
      identity.enabled,
    )
    if (saved && !disposed && current === generation) {
      cancel()
      state.notice = "Login availability updated."
    }
  }
  async function remove(identity: SystemPrincipalIdentity): Promise<void> {
    if (
      state.saving ||
      disposed ||
      !window.confirm(`Delete ${identity.key} and its password?`)
    )
      return
    const current = generation
    const saved = await mutate(
      "DELETE",
      identity,
      { expected_revision: identity.revision },
      true,
    )
    if (saved && !disposed && current === generation) {
      cancel()
      state.notice = "Login deleted."
    }
  }
  async function createPasswordLink(
    identity: SystemPrincipalIdentity,
  ): Promise<void> {
    if (disposed || state.saving) return
    cancel()
    state.notice = ""
    const request = mutation.begin()
    try {
      const response = await systemAdministration(
        `principals/${encodeURIComponent(owner)}/identities/${encodeURIComponent(identity.id)}/password-link`,
        "POST",
        { expected_revision: identity.revision },
        request.signal,
      )
      if (!request.current()) return
      if (response.status === 401) onAuthenticationLost()
      else if (response.status === 403) onSystemAccessChange("denied")
      else if (response.status === 409 || response.status === 404) {
        state.error =
          "This principal or login changed. Reload and create a new link."
        await onReload()
      } else if (response.status === 503) {
        state.error =
          "Password links require services.http.public_base_url to be configured."
      } else if (!response.ok)
        state.error = "Password link could not be created."
      else {
        const link = (await response.json()) as {
          url: string
          expires_at: string
        }
        if (request.current())
          state.passwordLink = {
            ...link,
            username: identity.key.slice("gatehouse:".length),
          }
      }
    } catch {
      if (request.current()) state.error = "Password link could not be created."
    } finally {
      request.finish()
    }
  }
  async function copyPasswordLink(): Promise<void> {
    const link = state.passwordLink
    if (!link) return
    state.error = state.notice = ""
    try {
      await navigator.clipboard.writeText(link.url)
      if (!disposed && state.passwordLink === link)
        state.notice = "Password link copied."
    } catch {
      if (!disposed && state.passwordLink === link)
        state.error = "Copy failed. Select and copy the link below."
    }
  }
  function stop() {
    generation++
    disposed = true
    mutation.stop()
    state.password = state.confirmation = ""
    state.passwordLink = null
  }
  return {
    state,
    reset,
    begin,
    cancel,
    save,
    setEnabled,
    remove,
    createPasswordLink,
    copyPasswordLink,
    stop,
  }
}
