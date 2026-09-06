import type { ActivityClient } from "../../app/activity"
import { createProjectSecret, fetchProjectSecret, fetchProjectSecrets, removeProjectSecret, updateProjectSecret, type ProjectSecret } from "../../app/project-secrets"
import type { Route } from "../../route"

type SecretRoute = Extract<Route, { kind: "project-secrets" | "project-secret-new" | "project-secret" }>
type Options = { activity: ActivityClient; onAuthenticationLost: () => void; onNavigate: (path: string, replace?: boolean) => void }

export function createProjectSecretsController({ activity, onAuthenticationLost, onNavigate }: Options) {
  const state = $state({ secrets: [] as ProjectSecret[], status: "checking" as "checking" | "ready" | "unavailable", active: null as ProjectSecret | null, creating: false, editing: false, saving: false, deleting: false, description: "", value: "", error: "" })
  let context: { workspaceID: string; projectID: string } | null = null
  let generation = 0
  let unsubscribe: (() => void) | undefined
  const listPath = (workspaceID: string, projectID: string) => `/app/wsp/${encodeURIComponent(workspaceID)}/prj/${encodeURIComponent(projectID)}/secrets`
  const detailPath = (workspaceID: string, projectID: string, secretID: string) => `${listPath(workspaceID, projectID)}/${encodeURIComponent(secretID)}`
  const current = (value: number) => value === generation

  async function loadList(showLoading: boolean, value: number): Promise<boolean> {
    if (context === null) return false
    const { workspaceID, projectID } = context
    if (showLoading) state.status = "checking"
    try {
      const response = await fetchProjectSecrets(workspaceID, projectID)
      if (!current(value)) return false
      if (response.status === 401) { onAuthenticationLost(); return false }
      if (!response.ok) throw new Error("project secrets could not be loaded")
      const secrets = await response.json() as ProjectSecret[]
      if (!current(value)) return false
      state.secrets = [...secrets].sort((left, right) => {
        const difference = new Date(right.updated_at).getTime() - new Date(left.updated_at).getTime()
        return Number.isFinite(difference) && difference !== 0 ? difference : right.id.localeCompare(left.id)
      })
      state.status = "ready"
      return true
    } catch { if (current(value)) state.status = "unavailable"; return false }
  }

  async function refresh(route: SecretRoute, value: number, showLoading = false): Promise<boolean> {
    if (!await loadList(showLoading, value) || !current(value) || context === null) return false
    if (route.kind !== "project-secret") return true
    const response = await fetchProjectSecret(context.workspaceID, context.projectID, route.secretID)
    if (!current(value) || context === null) return false
    if (response.status === 401) { onAuthenticationLost(); return false }
    if (response.status === 404) { clearSelection(); onNavigate(listPath(context.workspaceID, context.projectID), true); return true }
    if (!response.ok) throw new Error("project secret could not be loaded")
    state.active = await response.json() as ProjectSecret
    return true
  }

  function start(workspaceID: string, projectID: string, route: SecretRoute): () => void {
    stop(); context = { workspaceID, projectID }; const value = ++generation
    state.active = null; state.creating = false; state.editing = false; state.saving = false; state.deleting = false; state.description = ""; state.value = ""; state.error = ""
    if (route.kind === "project-secret-new") { state.creating = true; state.editing = true }
    void refresh(route, value, true)
    unsubscribe = activity.subscribe([{ name: "project-secret", topic: `${workspaceID}/${projectID}`, events: ["project_secret.*"] }], async ({ signal }) => { if (signal.aborted) return; if (!await refresh(route, value) || signal.aborted) throw new Error("project secrets refresh failed") })
    return stop
  }

  function stop(): void { generation += 1; unsubscribe?.(); unsubscribe = undefined; context = null; state.saving = false; state.deleting = false; state.value = "" }
  function clearSelection(): void { state.active = null; state.editing = false; state.value = "" }
  function startCreate(): void { if (context !== null) onNavigate(`${listPath(context.workspaceID, context.projectID)}/new`) }
  function startEdit(): void { if (state.active === null) return; state.description = state.active.description; state.value = ""; state.error = ""; state.editing = true }
  function cancelEdit(): void { if (state.saving || context === null) return; state.error = ""; state.value = ""; if (state.creating) { state.creating = false; state.editing = false; onNavigate(listPath(context.workspaceID, context.projectID)); return }; state.editing = false }

  async function save(): Promise<void> {
    if (context === null || state.description.trim() === "") { state.error = "Description is required."; return }
    if (state.creating && state.value === "") { state.error = "Value is required."; return }
    const { workspaceID, projectID } = context; const value = generation; const active = state.active; const input: { description: string; value?: string } = { description: state.description }; if (state.creating || state.value !== "") input.value = state.value
    state.error = ""; state.saving = true
    try {
      const response = state.creating ? await createProjectSecret(workspaceID, projectID, { description: input.description, value: input.value ?? "" }) : active === null ? undefined : await updateProjectSecret(workspaceID, projectID, active.id, input)
      if (response === undefined || !current(value) || context?.workspaceID !== workspaceID || context?.projectID !== projectID) return
      if (response.status === 401) { onAuthenticationLost(); return }
      if (!response.ok) throw new Error("project secret could not be saved")
      const saved = await response.json() as ProjectSecret
      if (!current(value)) return
      state.value = ""; state.active = saved; state.creating = false; state.editing = false; state.secrets = [saved, ...state.secrets.filter((secret) => secret.id !== saved.id)]; onNavigate(detailPath(workspaceID, projectID, saved.id))
    } catch { state.error = "The secret could not be saved. Try again." } finally { if (current(value)) state.saving = false }
  }

  async function remove(): Promise<void> {
    if (context === null || state.active === null || state.deleting || !window.confirm(`Remove ${state.active.description}?`)) return
    const { workspaceID, projectID } = context; const value = generation; const secret = state.active; state.deleting = true; state.error = ""
    try {
      const response = await removeProjectSecret(workspaceID, projectID, secret.id)
      if (!current(value) || context?.workspaceID !== workspaceID || context?.projectID !== projectID || state.active?.id !== secret.id) return
      if (response.status === 401) { onAuthenticationLost(); return }
      if (!response.ok) throw new Error("project secret could not be removed")
      clearSelection(); state.secrets = state.secrets.filter((candidate) => candidate.id !== secret.id); onNavigate(listPath(workspaceID, projectID))
    } catch { state.error = "The secret could not be removed. Try again." } finally { if (current(value)) state.deleting = false }
  }
  return { state, start, stop, startCreate, startEdit, cancelEdit, save, remove }
}
