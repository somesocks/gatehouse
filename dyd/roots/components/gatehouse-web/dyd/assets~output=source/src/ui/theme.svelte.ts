// These are shadcn-svelte's documented tokens, not a second token system.
export const themeTokens = [
  "background",
  "foreground",
  "card",
  "card-foreground",
  "popover",
  "popover-foreground",
  "primary",
  "primary-foreground",
  "secondary",
  "secondary-foreground",
  "muted",
  "muted-foreground",
  "accent",
  "accent-foreground",
  "destructive",
  "border",
  "input",
  "ring",
  "radius",
  "chart-1",
  "chart-2",
  "chart-3",
  "chart-4",
  "chart-5",
  "sidebar",
  "sidebar-foreground",
  "sidebar-primary",
  "sidebar-primary-foreground",
  "sidebar-accent",
  "sidebar-accent-foreground",
  "sidebar-border",
  "sidebar-ring",
] as const
export type ThemeValues = Partial<Record<(typeof themeTokens)[number], string>>
export type WorkspaceTheme = {
  mode?: "system" | "light" | "dark"
  light?: ThemeValues
  dark?: ThemeValues
}
const workspaceThemes = $state<Record<string, WorkspaceTheme>>({})

export function setWorkspaceTheme(
  workspaceID: string,
  theme: WorkspaceTheme | undefined,
) {
  if (theme === undefined) delete workspaceThemes[workspaceID]
  else workspaceThemes[workspaceID] = theme
}
export function workspaceTheme(workspaceID?: string): WorkspaceTheme {
  return workspaceID ? (workspaceThemes[workspaceID] ?? {}) : {}
}
