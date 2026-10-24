import type { Snippet } from "svelte"

export type ResourceDialogControls = {
  open: boolean
  onClose: () => void
  onClosed?: () => void
}

export type ResourceDialogOptions = ResourceDialogControls & {
  title: string
  viewKey: string
  pageKey?: string
  expanded?: boolean
  compact?: boolean
  busy?: boolean
  dismissible?: boolean
  includeClose?: boolean
  search?: Snippet
  footer?: Snippet
  initialFocus?: () => HTMLElement | undefined
}
