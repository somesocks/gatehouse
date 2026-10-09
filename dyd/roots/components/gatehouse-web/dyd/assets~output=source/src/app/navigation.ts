import type { Workspace } from "./access"
import { signOut } from "./auth"
import { updateChatSession } from "./chat"
import type { ApplicationRuntime } from "./runtime.svelte"
import type { NavigationOptions } from "../ui/navigation"

function account(runtime: ApplicationRuntime) {
  return {
    user: runtime.auth.state.claims?.principal.name ?? "User",
    onSignOut: async () => {
      try {
        await signOut()
      } finally {
        runtime.requireLogin()
      }
    },
  }
}

export function workspaceNavigation(
  runtime: ApplicationRuntime,
  workspace: Workspace,
  active?: string,
): NavigationOptions {
  const base = `/app/wsp/${encodeURIComponent(workspace.id)}`
  return {
    title: workspace.name ?? workspace.id,
    items: [
      { label: "Overview", href: base, active: active === undefined },
      { label: "Chats", href: `${base}/ses`, active: active === "chats" },
      { label: "Projects", href: `${base}/prj`, active: active === "projects" },
      { label: "Groups", href: `${base}/grp`, active: active === "groups" },
      ...(runtime.access.state.systemAccess === "available"
        ? [{ label: "System", href: "/app/system", external: true }]
        : []),
    ],
    ...account(runtime),
  }
}

export function systemNavigation(
  runtime: ApplicationRuntime,
  active: string,
): NavigationOptions {
  const items = [
    ["overview", "Overview", ""],
    ["principals", "Principals", "/principals"],
    ["grants", "System grants", "/grants"],
    ["agent-providers", "Agent providers", "/agent-providers"],
    ["agent-models", "Agent models", "/agent-models"],
    ["storage-providers", "Storage providers", "/storage-providers"],
    [
      "workspace-agent-bindings",
      "Workspace agent bindings",
      "/workspace-agent-bindings",
    ],
    [
      "workspace-storage-bindings",
      "Workspace storage bindings",
      "/workspace-storage-bindings",
    ],
  ]
  return {
    title: "System",
    items: items.map(([key, label, path]) => ({
      label,
      href: `/app/system${path}`,
      active: active === key,
    })),
    ...account(runtime),
  }
}

export function sessionViews(
  workspaceID: string,
  sessionID: string,
  active: string,
) {
  const base = `/app/wsp/${encodeURIComponent(workspaceID)}/ses/${encodeURIComponent(sessionID)}`
  return ["chat", "files", "notes", "tasks", "secrets"].map((view) => ({
    label: view[0].toUpperCase() + view.slice(1),
    href: view === "chat" ? base : `${base}/${view}`,
    active: view === active,
  }))
}

export async function renameSession(
  runtime: ApplicationRuntime,
  workspaceID: string,
  sessionID: string,
  name: string,
  onRenamed?: (name: string) => void,
) {
  const response = await updateChatSession(workspaceID, sessionID, { name })
  if (response.status === 401) {
    runtime.requireLogin()
    return
  }
  if (!response.ok) throw new Error("The chat could not be renamed. Try again.")
  const updated = (await response.json()) as { name?: string }
  onRenamed?.(updated.name ?? name)
}
