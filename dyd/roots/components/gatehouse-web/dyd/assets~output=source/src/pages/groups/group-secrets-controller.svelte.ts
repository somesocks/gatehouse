import type { ActivityClient } from "../../app/activity"
import {
  createGroupSecret,
  fetchGroupSecret,
  fetchGroupSecrets,
  removeGroupSecret,
  updateGroupSecret,
  type GroupSecret,
  type GroupSecretPage,
} from "../../app/group-secrets"
import { createDirectoryController } from "../../utils/directory-controller.svelte"

export type GroupSecretsController = ReturnType<typeof createGroupSecretsController>

export function createGroupSecretsController({ activity, onAuthenticationLost, onAccessLost }: {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onAccessLost: () => void
}) {
  let workspace = ""
  let group = ""
  let generation = 0
  let operation: AbortController | undefined
  let metadata: AbortController | undefined
  let unsubscribe: (() => void) | undefined
  const state = $state({
    active: false,
    canManage: false,
    dialog: null as "create" | "edit" | "remove" | null,
    snapshot: null as GroupSecret | null,
    description: "",
    value: "",
    saving: false,
    reloading: false,
    error: "",
  })
  const current = (value: number, signal: AbortSignal) => state.active && generation === value && !signal.aborted

  const directory = createDirectoryController<GroupSecret>({
    onAuthenticationLost,
    async fetchPage(name, cursor, _status, signal) {
      const value = generation
      const response = await fetchGroupSecrets(workspace, group, name, cursor, signal)
      const body = response.ok ? await response.json() as GroupSecretPage : undefined
      signal.throwIfAborted()
      if (!current(value, signal)) throw new DOMException("Obsolete group", "AbortError")
      if (response.status === 404) {
        loseAccess()
        return { response }
      }
      if (body) {
        state.canManage = body.can_manage_secrets
        if (!state.canManage && state.dialog) cancelDialog()
      }
      return { response, items: body?.secrets, next: body?.next_cursor }
    },
  })

  function clearDialog() {
    metadata?.abort()
    state.dialog = null
    state.snapshot = null
    state.description = ""
    state.value = ""
    state.reloading = false
  }
  function cancelDialog() {
    operation?.abort()
    state.saving = false
    state.error = ""
    clearDialog()
  }
  function loseAccess() {
    stop()
    directory.reset()
    onAccessLost()
  }
  function stop() {
    generation++
    state.active = false
    state.canManage = false
    cancelDialog()
    directory.stop()
    unsubscribe?.()
    unsubscribe = undefined
  }
  function start(workspaceID: string, groupID: string) {
    stop()
    workspace = workspaceID
    group = groupID
    directory.reset()
    state.active = true
    const value = generation
    void directory.search()
    unsubscribe = activity.subscribe([
      { name: "group-secrets", topic: `${workspace}/${group}`, events: ["group_secret.*", "group.*", "group_member.*"] },
      { name: "group-secrets-access", topic: workspace, events: ["workspace_grant.*", "group.*", "group_member.*"] },
    ], async ({ signal }) => {
      if (!current(value, signal)) return
      if (!(await directory.reload()) && current(value, signal))
        throw new Error("Group secrets could not be refreshed.")
    })
    void activity.poll()
    return stop
  }
  function begin(mode: "create" | "edit" | "remove", secret?: GroupSecret) {
    if (!state.active || !state.canManage || state.saving) return
    if (mode !== "create" && !secret) return
    cancelDialog()
    state.dialog = mode
    // Freeze the revision at opening; background refreshes never replace drafts.
    state.snapshot = secret ? { ...secret } : null
    state.description = secret?.description ?? ""
  }
  async function reloadSecret() {
    const snapshot = state.snapshot
    if (!snapshot || state.saving || state.reloading) return
    metadata?.abort()
    const own = new AbortController()
    metadata = own
    const value = generation
    state.reloading = true
    state.error = ""
    try {
      const response = await fetchGroupSecret(workspace, group, snapshot.id, own.signal)
      if (!current(value, own.signal)) return
      if (response.status === 401) { onAuthenticationLost(); return }
      if (response.status === 404) {
        clearDialog()
        state.error = "This secret is no longer available."
        void directory.reload()
        return
      }
      if (!response.ok) throw new Error("Secret could not be refreshed.")
      const latest = await response.json() as GroupSecret
      if (!current(value, own.signal) || state.snapshot !== snapshot) return
      state.snapshot = latest
      state.description = latest.description
      state.value = ""
    } catch (error) {
      if (current(value, own.signal)) state.error = error instanceof Error ? error.message : "Secret could not be refreshed."
    } finally {
      if (current(value, own.signal)) state.reloading = false
    }
  }
  async function save() {
    const { dialog, snapshot, description, value } = state
    if (!state.active || !state.canManage || !dialog || state.saving || state.reloading) return false
    if (dialog !== "remove" && (!description.trim() || (dialog === "create" && !value))) return false
    operation?.abort()
    const own = new AbortController()
    operation = own
    const selected = generation
    state.saving = true
    state.error = ""
    try {
      const response = dialog === "create"
        ? await createGroupSecret(workspace, group, description.trim(), value, own.signal)
        : dialog === "remove"
          ? await removeGroupSecret(workspace, group, snapshot!, own.signal)
          : await updateGroupSecret(workspace, group, snapshot!, description.trim(), value, own.signal)
      const message = response.ok ? "" : await response.text()
      if (!current(selected, own.signal)) return false
      if (response.status === 401) { onAuthenticationLost(); return false }
      if (response.status === 403) {
        state.canManage = false
        directory.stop() // Do not let an older response restore stale controls.
        clearDialog()
        throw new Error("Group manager access is required.")
      }
      if (!response.ok) throw new Error(message.trim() || "Secret could not be saved.")
      clearDialog() // Clear credential drafts even when refreshing metadata fails.
      if (!(await directory.reload()) && current(selected, own.signal))
        state.error = "Saved, but secrets could not be refreshed."
      void activity.poll()
      return true
    } catch (error) {
      if (current(selected, own.signal)) state.error = error instanceof Error ? error.message : "Secret could not be saved."
      return false
    } finally {
      if (current(selected, own.signal)) state.saving = false
    }
  }
  return { state, directory, start, stop, begin, cancelDialog, reloadSecret, save }
}
