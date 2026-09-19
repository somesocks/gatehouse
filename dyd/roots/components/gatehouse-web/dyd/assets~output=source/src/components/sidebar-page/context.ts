import { getContext } from "svelte"

export type SidebarPageState = {
  drawer: HTMLDialogElement | undefined
}

export const sidebarPageContext = Symbol("sidebar-page")

export function useSidebarPage(): SidebarPageState {
  const state = getContext<SidebarPageState>(sidebarPageContext)
  if (state === undefined)
    throw new Error(
      "SidebarPage components must be nested under SidebarPage.Root",
    )
  return state
}
