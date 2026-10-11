// A dialog edits a draft. The caller's reference changes only on confirmation.
export function createReferenceSelection() {
  const state = $state({ initial: "", initialLabel: "", draft: "", label: "" })
  function begin(value: string, label: string) {
    state.initial = value
    state.draft = value
    state.label = label
    state.initialLabel = label
  }
  function choose(value: string, label: string) {
    state.draft = value
    state.label = label
  }
  function cancel() {
    state.draft = state.initial
    state.label = state.initialLabel
  }
  function confirm(): string {
    state.initial = state.draft
    state.initialLabel = state.label
    return state.draft
  }
  return { state, begin, choose, cancel, confirm }
}
