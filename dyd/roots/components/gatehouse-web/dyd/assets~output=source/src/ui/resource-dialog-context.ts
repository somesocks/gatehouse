import { getContext, setContext, type Snippet } from "svelte"

export type ResourceFooter = {
  owner: symbol
  children: Snippet
  includeClose: boolean
}
export type ResourceDialogContext = {
  footer?: ResourceFooter
  resetScroll: () => void
}
const key = Symbol("resource-dialog")

export function provideResourceDialog(context: ResourceDialogContext) {
  setContext(key, context)
}

export function resourceDialog(): ResourceDialogContext | undefined {
  return getContext(key)
}
