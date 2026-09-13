import type { ActivityClient } from "../../app/activity"
import {
  createSessionSecret,
  fetchSessionSecret,
  fetchSessionSecrets,
  removeSessionSecret,
  updateSessionSecret,
  type SessionSecret,
} from "../../app/session-secrets"
import type { Route } from "../../route"

type SessionSecretsRoute = Extract<
  Route,
  { kind: "session-secrets" | "session-secret-new" | "session-secret" }
>
type SessionSecretStatus = "checking" | "ready" | "unavailable"

type SessionSecretsControllerOptions = {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onNavigate: (path: string, replace?: boolean) => void
}

export function createSessionSecretsController({
  activity,
  onAuthenticationLost,
  onNavigate,
}: SessionSecretsControllerOptions) {
  const state = $state({
    secrets: [] as SessionSecret[],
    status: "checking" as SessionSecretStatus,
    active: null as SessionSecret | null,
    creating: false,
    editing: false,
    saving: false,
    deleting: false,
    description: "",
    value: "",
    error: "",
  })
  let context: {
    workspaceID: string
    sessionID: string
    signal: AbortSignal
  } | null = null
  let generation = 0
  let unsubscribe: (() => void) | undefined

  function listPath(workspaceID: string, sessionID: string): string {
    return `/app/wsp/${encodeURIComponent(workspaceID)}/ses/${encodeURIComponent(sessionID)}/secrets`
  }

  function detailPath(
    workspaceID: string,
    sessionID: string,
    secretID: string,
  ): string {
    return `${listPath(workspaceID, sessionID)}/${encodeURIComponent(secretID)}`
  }

  function routeSecretID(route: SessionSecretsRoute): string | null {
    return route.kind === "session-secret" ? route.secretID : null
  }

  function isCurrent(currentGeneration: number): boolean {
    return currentGeneration === generation
  }

  async function loadSecrets(
    showLoading: boolean,
    currentGeneration: number,
  ): Promise<boolean> {
    if (context === null) {
      return false
    }
    const { workspaceID, sessionID } = context
    if (showLoading) {
      state.status = "checking"
    }
    try {
      const response = await fetchSessionSecrets(
        workspaceID,
        sessionID,
        context.signal,
      )
      if (!isCurrent(currentGeneration)) {
        return false
      }
      if (response.status === 401) {
        onAuthenticationLost()
        return false
      }
      if (!response.ok) {
        throw new Error("session secrets could not be loaded")
      }
      const secrets = (await response.json()) as SessionSecret[]
      if (!isCurrent(currentGeneration)) {
        return false
      }
      state.secrets = [...secrets].sort((left, right) => {
        const difference =
          new Date(right.updated_at).getTime() -
          new Date(left.updated_at).getTime()
        return Number.isFinite(difference) && difference !== 0
          ? difference
          : right.id.localeCompare(left.id)
      })
      state.status = "ready"
      return true
    } catch {
      if (isCurrent(currentGeneration)) {
        state.status = "unavailable"
      }
      return false
    }
  }

  async function loadSecret(
    secretID: string,
    currentGeneration: number,
  ): Promise<SessionSecret | null> {
    if (context === null) {
      return null
    }
    const { workspaceID, sessionID } = context
    const response = await fetchSessionSecret(
      workspaceID,
      sessionID,
      secretID,
      context.signal,
    )
    if (!isCurrent(currentGeneration)) {
      return null
    }
    if (response.status === 401) {
      onAuthenticationLost()
      return null
    }
    if (response.status === 404) {
      return null
    }
    if (!response.ok) {
      throw new Error("session secret could not be loaded")
    }
    const loaded = (await response.json()) as SessionSecret
    return isCurrent(currentGeneration) &&
      context?.workspaceID === workspaceID &&
      context?.sessionID === sessionID
      ? loaded
      : null
  }

  async function refresh(
    route: SessionSecretsRoute,
    currentGeneration: number,
    showLoading = false,
  ): Promise<boolean> {
    if (
      !(await loadSecrets(showLoading, currentGeneration)) ||
      !isCurrent(currentGeneration) ||
      context === null
    ) {
      return false
    }
    const secretID = routeSecretID(route)
    if (secretID === null) {
      return true
    }
    const secret = await loadSecret(secretID, currentGeneration)
    if (!isCurrent(currentGeneration) || context === null) {
      return false
    }
    if (secret === null) {
      clearSelection()
      onNavigate(listPath(context.workspaceID, context.sessionID), true)
      return true
    }
    state.active = secret
    return true
  }

  function activateCreate(): void {
    state.active = null
    state.creating = true
    state.editing = true
    state.description = ""
    state.value = ""
    state.error = ""
  }

  function start(
    workspaceID: string,
    sessionID: string,
    route: SessionSecretsRoute,
    signal: AbortSignal,
  ): () => void {
    stop()
    context = { workspaceID, sessionID, signal }
    const currentGeneration = ++generation
    state.active = null
    state.creating = false
    state.editing = false
    state.saving = false
    state.deleting = false
    state.description = ""
    state.value = ""
    state.error = ""
    if (route.kind === "session-secret-new") {
      activateCreate()
    }
    void refresh(route, currentGeneration, true)
    unsubscribe = activity.subscribe(
      [
        {
          name: "session-secret",
          topic: `${workspaceID}/${sessionID}`,
          events: ["session_secret.*"],
        },
      ],
      async ({ signal }) => {
        if (signal.aborted) {
          return
        }
        if (!(await refresh(route, currentGeneration)) || signal.aborted) {
          throw new Error("session secrets refresh failed")
        }
      },
    )
    return stop
  }

  function stop(): void {
    generation += 1
    unsubscribe?.()
    unsubscribe = undefined
    context = null
    state.saving = false
    state.deleting = false
    state.value = ""
  }

  function startCreate(): void {
    if (context !== null) {
      onNavigate(`${listPath(context.workspaceID, context.sessionID)}/new`)
    }
  }

  function startEdit(): void {
    if (state.active === null) {
      return
    }
    state.description = state.active.description
    state.value = ""
    state.error = ""
    state.editing = true
  }

  function cancelEdit(): void {
    if (state.saving || context === null) {
      return
    }
    state.error = ""
    state.value = ""
    if (state.creating) {
      state.creating = false
      state.editing = false
      onNavigate(listPath(context.workspaceID, context.sessionID))
      return
    }
    state.editing = false
  }

  async function save(): Promise<void> {
    if (context === null || state.description.trim() === "") {
      state.error = "Description is required."
      return
    }
    if (state.creating && state.value === "") {
      state.error = "Value is required."
      return
    }
    const { workspaceID, sessionID } = context
    const currentGeneration = generation
    const creating = state.creating
    const active = state.active
    const input: { description: string; value?: string } = {
      description: state.description,
    }
    if (creating || state.value !== "") {
      input.value = state.value
    }
    state.error = ""
    state.saving = true
    try {
      const response = creating
        ? await createSessionSecret(
            workspaceID,
            sessionID,
            { description: input.description, value: input.value ?? "" },
            context.signal,
          )
        : active === null
          ? undefined
          : await updateSessionSecret(
              workspaceID,
              sessionID,
              active.id,
              input,
              context.signal,
            )
      if (
        response === undefined ||
        !isCurrent(currentGeneration) ||
        context?.workspaceID !== workspaceID ||
        context?.sessionID !== sessionID
      ) {
        return
      }
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (!response.ok) {
        throw new Error("session secret could not be saved")
      }
      const saved = (await response.json()) as SessionSecret
      if (!isCurrent(currentGeneration)) {
        return
      }
      state.value = ""
      state.active = saved
      state.creating = false
      state.editing = false
      state.secrets = [
        saved,
        ...state.secrets.filter((secret) => secret.id !== saved.id),
      ]
      onNavigate(detailPath(workspaceID, sessionID, saved.id))
    } catch {
      state.error = "The secret could not be saved. Try again."
    } finally {
      if (isCurrent(currentGeneration)) {
        state.saving = false
      }
    }
  }

  async function remove(): Promise<void> {
    if (
      context === null ||
      state.active === null ||
      state.deleting ||
      !window.confirm(`Remove ${state.active.description}?`)
    ) {
      return
    }
    const { workspaceID, sessionID } = context
    const currentGeneration = generation
    const secret = state.active
    state.deleting = true
    state.error = ""
    try {
      const response = await removeSessionSecret(
        workspaceID,
        sessionID,
        secret.id,
        context.signal,
      )
      if (
        !isCurrent(currentGeneration) ||
        context?.workspaceID !== workspaceID ||
        context?.sessionID !== sessionID ||
        state.active?.id !== secret.id
      ) {
        return
      }
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (!response.ok) {
        throw new Error("session secret could not be removed")
      }
      clearSelection()
      state.secrets = state.secrets.filter(
        (candidate) => candidate.id !== secret.id,
      )
      onNavigate(listPath(workspaceID, sessionID))
    } catch {
      state.error = "The secret could not be removed. Try again."
    } finally {
      if (isCurrent(currentGeneration)) {
        state.deleting = false
      }
    }
  }

  function clearSelection(): void {
    state.active = null
    state.editing = false
    state.value = ""
  }

  return {
    state,
    start,
    stop,
    startCreate,
    startEdit,
    cancelEdit,
    save,
    remove,
  }
}
