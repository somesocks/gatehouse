export type NavigationItem = {
  label: string
  href: string
  icon?: NavigationIcon
  active?: boolean
  external?: boolean
}
export type NavigationIcon =
  | "overview"
  | "chats"
  | "projects"
  | "groups"
  | "principals"
  | "grants"
  | "agent-providers"
  | "agent-models"
  | "storage-providers"
  | "workspace-agent-bindings"
  | "workspace-storage-bindings"
export type SidebarNavigationItem = NavigationItem & { icon: NavigationIcon }
export type NavigationOptions = {
  title?: string
  sections: NavigationSection[]
  workspaceSwitcher?: WorkspaceSwitcherOptions
  user?: { name: string }
  onSignOut?: () => void | Promise<void>
}
export type SidebarUserOptions = {
  user: { name: string }
  onSignOut: () => void | Promise<void>
}
export type NavigationSection = {
  title?: string
  items: SidebarNavigationItem[]
}
export type WorkspaceSwitcherOptions = {
  currentID: string | null
  currentName: string
  workspaces: { id: string; name: string }[]
  onSelect: (workspaceID: string) => void
  administration?: { active: boolean; onSelect: () => void }
}
export type BreadcrumbItem = { label: string; href?: string }
