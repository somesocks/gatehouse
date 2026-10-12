// Request lifecycle only. Resource controllers retain their native snapshots,
// transports, drafts, and response models.
export function createLoadState() {
  const state = $state({ hasLoaded: false, pending: false, error: "" })
  let request: AbortController | undefined
  let sequence = 0
  function stop() {
    sequence++
    request?.abort()
    request = undefined
    state.pending = false
  }
  function reset() {
    stop()
    state.hasLoaded = false
    state.error = ""
  }
  function begin(parent?: AbortSignal) {
    request?.abort()
    const controller = new AbortController()
    request = controller
    const value = ++sequence
    const signal = parent
      ? AbortSignal.any([parent, controller.signal])
      : controller.signal
    state.pending = true
    state.error = ""
    const current = () => value === sequence && !signal.aborted
    return {
      signal,
      current,
      commit(action: () => void) {
        if (!current()) return false
        action()
        state.hasLoaded = true
        state.error = ""
        return true
      },
      fail(error: unknown) {
        if (current())
          state.error = error instanceof Error ? error.message : String(error)
      },
      finish() {
        if (value === sequence) state.pending = false
      },
    }
  }
  return {
    state,
    begin,
    stop,
    reset,
    get loading() {
      return !state.hasLoaded && !state.error
    },
    get reloading() {
      return state.hasLoaded && state.pending
    },
    get status(): "checking" | "ready" | "unavailable" {
      return state.hasLoaded
        ? "ready"
        : state.error
          ? "unavailable"
          : "checking"
    },
  }
}

export function createAsyncResource<T>() {
  const lifecycle = createLoadState()
  const snapshot = $state({ value: undefined as T | undefined })
  let identity: string | undefined
  function context(key: string) {
    if (key === identity) return false
    identity = key
    lifecycle.reset()
    snapshot.value = undefined
    return true
  }
  async function load(
    fetch: (signal: AbortSignal) => Promise<T>,
    parent?: AbortSignal,
  ): Promise<boolean> {
    const request = lifecycle.begin(parent)
    try {
      const value = await fetch(request.signal)
      return request.commit(() => {
        snapshot.value = value
      })
    } catch (error) {
      request.fail(error)
      return false
    } finally {
      request.finish()
    }
  }
  return {
    lifecycle,
    snapshot,
    context,
    load,
    stop: lifecycle.stop,
    get value() {
      return snapshot.value
    },
  }
}
