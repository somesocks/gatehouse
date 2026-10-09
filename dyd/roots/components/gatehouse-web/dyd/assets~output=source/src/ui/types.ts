import type { HTMLAttributes } from "svelte/elements"

// Presentation is configured through component props, never CSS escape hatches.
export type Attributes<T extends EventTarget = HTMLElement> = Omit<
  HTMLAttributes<T>,
  "class" | "style"
>
export type Gap = "none" | "tight" | "small" | "normal" | "large"
export const gaps = {
  none: "gap-0",
  tight: "gap-1",
  small: "gap-2",
  normal: "gap-4",
  large: "gap-6",
} as const
export type DialogHandle = {
  readonly open: boolean
  showModal(): void
  close(): void
}
