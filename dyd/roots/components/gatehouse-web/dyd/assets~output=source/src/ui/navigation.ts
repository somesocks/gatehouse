export type NavigationItem = {
  label: string
  href: string
  active?: boolean
  external?: boolean
}
export type NavigationOptions = {
  title?: string
  sections: NavigationSection[]
  workspaceSwitcher?: WorkspaceSwitcherOptions
  user?: string
  onSignOut?: () => void | Promise<void>
}
export type NavigationSection = {
  title?: string
  items: NavigationItem[]
}
export type WorkspaceSwitcherOptions = {
  currentID: string
  currentName: string
  workspaces: { id: string; name: string }[]
  onSelect: (workspaceID: string) => void
}
export type BreadcrumbItem = { label: string; href?: string }
