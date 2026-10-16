import { fetchWorkspaces, type Workspace } from "./access"
import type { ActivityClient } from "./activity"
import { fetchSystemGrants } from "./system"

export type WorkspaceStatus = "checking" | "ready" | "empty" | "unavailable"
export type SystemAccessStatus =
  | "checking"
  | "available"
  | "denied"
  | "unavailable"

type AccessOptions = {
  activity: ActivityClient
  onAuthenticationLost: () => void
  transport?: {
    system: typeof fetchSystemGrants
    workspaces: typeof fetchWorkspaces
  }
}

export function createAccess({
  activity,
  onAuthenticationLost,
  transport = { system: fetchSystemGrants, workspaces: fetchWorkspaces },
}: AccessOptions) {
  const state = $state<{
    workspaceStatus: WorkspaceStatus
    workspaces: Workspace[]
    systemAccess: SystemAccessStatus
    reloading: boolean
    reloadError: string
  }>({
    workspaceStatus: "checking",
    workspaces: [],
    systemAccess: "checking",
    reloading: false,
    reloadError: "",
  })
  let refreshPromise: Promise<boolean> | null = null
  let unsubscribe: (() => void) | undefined
  let generation = 0
  let controller: AbortController | undefined
  let principal: string | undefined

  async function refresh(): Promise<boolean> {
    if (refreshPromise !== null) {
      return await refreshPromise
    }
    const currentGeneration = generation
    controller?.abort()
    controller = new AbortController()
    const signal = controller.signal
    const loadedWorkspace =
      state.workspaceStatus === "ready" || state.workspaceStatus === "empty"
    const loadedSystem =
      state.systemAccess === "available" || state.systemAccess === "denied"
    if (!loadedWorkspace) state.workspaceStatus = "checking"
    if (!loadedSystem) state.systemAccess = "checking"
    state.reloading = loadedWorkspace || loadedSystem
    state.reloadError = ""
    const pending = (async () => {
      try {
        const [systemResult, workspaceResult] = await Promise.allSettled([
          transport.system(signal),
          transport.workspaces(signal),
        ])
        if (currentGeneration !== generation) return false
        const systemResponse =
          systemResult.status === "fulfilled" ? systemResult.value : null
        if (!systemResponse) {
          if (!loadedSystem) updateSystemAccess("unavailable")
          state.reloadError = "System access could not be refreshed."
        } else {
          if (systemResponse.status === 401) {
            onAuthenticationLost()
            return false
          }
          if (systemResponse.status === 403) updateSystemAccess("denied")
          else if (systemResponse.ok) updateSystemAccess("available")
          else {
            if (!loadedSystem) updateSystemAccess("unavailable")
            state.reloadError = "System access could not be refreshed."
          }
        }

        if (workspaceResult.status === "rejected") throw workspaceResult.reason
        const workspaceResponse = workspaceResult.value
        if (currentGeneration !== generation) return false
        if (workspaceResponse.status === 401) {
          onAuthenticationLost()
          return false
        }
        if (workspaceResponse.status === 403) {
          state.workspaces = []
          state.workspaceStatus = "empty"
          return state.reloadError === ""
        }
        if (!workspaceResponse.ok) {
          if (!loadedWorkspace) state.workspaceStatus = "unavailable"
          state.reloadError = "Workspace access could not be refreshed."
          return false
        }
        const workspaces = (await workspaceResponse.json()) as Workspace[]
        if (currentGeneration !== generation) return false
        state.workspaces = workspaces
        state.workspaceStatus =
          state.workspaces.length === 0 ? "empty" : "ready"
        return state.reloadError === ""
      } catch {
        if (currentGeneration === generation) {
          if (!loadedWorkspace) state.workspaceStatus = "unavailable"
          if (state.systemAccess === "checking")
            updateSystemAccess("unavailable")
          state.reloadError = "Access could not be refreshed."
        }
        return false
      }
    })()
    refreshPromise = pending
    try {
      return await pending
    } finally {
      if (refreshPromise === pending) {
        refreshPromise = null
        state.reloading = false
      }
    }
  }

  function start(principalID: string): () => void {
    if (principal !== principalID) {
      clear()
      principal = principalID
    }
    stop()
    unsubscribe = activity.subscribe(
      [
        {
          name: "access",
          topic: principalID,
          events: [
            "workspace_grant.*",
            "group.*",
            "group_member.*",
            "system_grant.*",
          ],
        },
      ],
      async () => {
        if (!(await refresh())) {
          throw new Error("access refresh failed")
        }
      },
    )
    return stop
  }

  function stop(): void {
    unsubscribe?.()
    unsubscribe = undefined
  }

  function clear(): void {
    generation += 1
    controller?.abort()
    principal = undefined
    stop()
    state.workspaceStatus = "checking"
    state.workspaces = []
    state.systemAccess = "checking"
    state.reloading = false
    state.reloadError = ""
    refreshPromise = null
  }

  function setSystemAccess(
    next: Exclude<SystemAccessStatus, "checking">,
  ): void {
    if (next === "denied") {
      generation++
      controller?.abort()
      refreshPromise = null
      state.reloading = false
    }
    updateSystemAccess(next)
  }

  function updateSystemAccess(
    next: Exclude<SystemAccessStatus, "checking">,
  ): void {
    state.systemAccess = next
  }

  return { state, refresh, start, stop, clear, setSystemAccess }
}
