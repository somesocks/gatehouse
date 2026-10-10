import type { Workspace } from "./access"
import { signOut } from "./auth"
import { updateChatSession } from "./chat"
import type { ApplicationRuntime } from "./runtime.svelte"
import type { NavigationIcon, NavigationOptions } from "../ui/navigation"

function workspaceSwitcher(
  runtime: ApplicationRuntime,
  currentID: string | null,
  currentName: string,
  administrationActive = false,
) {
  return {
    currentID,
    currentName,
    workspaces: runtime.access.state.workspaces.map((item) => ({
      id: item.id,
      name: item.name ?? "Workspace",
    })),
    onSelect: (workspaceID: string) =>
      runtime.navigate(`/app/wsp/${encodeURIComponent(workspaceID)}`),
    administration:
      runtime.access.state.systemAccess === "available"
        ? {
            active: administrationActive,
            onSelect: () => runtime.navigate("/app/system"),
          }
        : undefined,
  }
}

function account(runtime: ApplicationRuntime) {
  return {
    user: {
      name: runtime.auth.state.claims?.principal.name ?? "User",
    },
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
    workspaceSwitcher: workspaceSwitcher(
      runtime,
      workspace.id,
      workspace.name ?? "Workspace",
    ),
    sections: [
      {
        title: "Workspace",
        items: [
          {
            label: "Overview",
            href: base,
            icon: "overview",
            active: active === undefined,
          },
          {
            label: "Chats",
            href: `${base}/ses`,
            icon: "chats",
            active: active === "chats",
          },
          {
            label: "Projects",
            href: `${base}/prj`,
            icon: "projects",
            active: active === "projects",
          },
          {
            label: "Groups",
            href: `${base}/grp`,
            icon: "groups",
            active: active === "groups",
          },
        ],
      },
    ],
    ...account(runtime),
  }
}

export function systemNavigation(
  runtime: ApplicationRuntime,
  active: string,
): NavigationOptions {
  const items: {
    key: string
    label: string
    path: string
    icon: NavigationIcon
  }[] = [
    {
      key: "overview",
      label: "Overview",
      path: "",
      icon: "overview",
    },
    {
      key: "principals",
      label: "Principals",
      path: "/principals",
      icon: "principals",
    },
    {
      key: "grants",
      label: "System grants",
      path: "/grants",
      icon: "grants",
    },
    {
      key: "agent-providers",
      label: "Agent providers",
      path: "/agent-providers",
      icon: "agent-providers",
    },
    {
      key: "agent-models",
      label: "Agent models",
      path: "/agent-models",
      icon: "agent-models",
    },
    {
      key: "storage-providers",
      label: "Storage providers",
      path: "/storage-providers",
      icon: "storage-providers",
    },
    {
      key: "workspaces",
      label: "Workspaces",
      path: "/workspaces",
      icon: "workspaces",
    },
    {
      key: "workspace-agent-bindings",
      label: "Workspace agent bindings",
      path: "/workspace-agent-bindings",
      icon: "workspace-agent-bindings",
    },
    {
      key: "workspace-storage-bindings",
      label: "Workspace storage bindings",
      path: "/workspace-storage-bindings",
      icon: "workspace-storage-bindings",
    },
  ]
  return {
    title: "System administration",
    workspaceSwitcher: workspaceSwitcher(
      runtime,
      null,
      "System administration",
      true,
    ),
    sections: [
      {
        items: items.map(({ key, label, path, icon }) => ({
          label,
          href: `/app/system${path}`,
          icon,
          active: active === key,
        })),
      },
    ],
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
