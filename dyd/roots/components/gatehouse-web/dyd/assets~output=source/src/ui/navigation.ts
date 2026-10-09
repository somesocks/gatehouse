export type NavigationItem = {
  label: string
  href: string
  active?: boolean
  external?: boolean
}
export type NavigationOptions = {
  title: string
  items: NavigationItem[]
  user?: string
  onSignOut?: () => void | Promise<void>
}
export type BreadcrumbItem = { label: string; href?: string }
