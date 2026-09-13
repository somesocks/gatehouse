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
}

export function createAccess({
  activity,
  onAuthenticationLost,
}: AccessOptions) {
  const state = $state<{
    workspaceStatus: WorkspaceStatus
    workspaces: Workspace[]
    systemAccess: SystemAccessStatus
  }>({
    workspaceStatus: "checking",
    workspaces: [],
    systemAccess: "checking",
  })
  let refreshPromise: Promise<boolean> | null = null
  let unsubscribe: (() => void) | undefined
  let generation = 0

  async function refresh(): Promise<boolean> {
    if (refreshPromise !== null) {
      return await refreshPromise
    }
    const currentGeneration = generation
    state.workspaceStatus = "checking"
    state.systemAccess = "checking"
    const pending = (async () => {
      try {
        const systemResponse = await fetchSystemGrants()
        if (currentGeneration !== generation) return false
        if (systemResponse.status === 401) {
          onAuthenticationLost()
          return false
        }
        updateSystemAccess(
          systemResponse.status === 403
            ? "denied"
            : systemResponse.ok
              ? "available"
              : "unavailable",
        )

        const workspaceResponse = await fetchWorkspaces()
        if (currentGeneration !== generation) return false
        if (workspaceResponse.status === 401) {
          onAuthenticationLost()
          return false
        }
        if (!workspaceResponse.ok) {
          state.workspaceStatus = "unavailable"
          return false
        }
        state.workspaces = (await workspaceResponse.json()) as Workspace[]
        state.workspaceStatus =
          state.workspaces.length === 0 ? "empty" : "ready"
        return true
      } catch {
        if (currentGeneration === generation) {
          state.workspaceStatus = "unavailable"
          updateSystemAccess("unavailable")
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
      }
    }
  }

  function start(principalID: string): () => void {
    stop()
    unsubscribe = activity.subscribe(
      [
        {
          name: "access",
          topic: principalID,
          events: ["workspace_grant.*", "group_member.*", "system_grant.*"],
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
    stop()
    state.workspaceStatus = "checking"
    state.workspaces = []
    state.systemAccess = "checking"
  }

  function setSystemAccess(
    next: Exclude<SystemAccessStatus, "checking">,
  ): void {
    updateSystemAccess(next)
  }

  function updateSystemAccess(
    next: Exclude<SystemAccessStatus, "checking">,
  ): void {
    state.systemAccess = next
  }

  return { state, refresh, start, stop, clear, setSystemAccess }
}
