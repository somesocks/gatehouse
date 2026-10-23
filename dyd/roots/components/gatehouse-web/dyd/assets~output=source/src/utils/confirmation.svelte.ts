export type ConfirmationRequest = {
  title: string
  description: string
  confirmLabel: string
  onConfirm: () => string | void | Promise<string | void>
}

export function createConfirmation() {
  const state = $state({
    request: null as ConfirmationRequest | null,
    pending: false,
    error: "",
  })

  function reset() {
    state.request = null
    state.pending = false
    state.error = ""
  }

  return {
    state,
    request(request: ConfirmationRequest) {
      if (state.request || state.pending) return
      state.error = ""
      state.request = { ...request }
    },
    reset,
    cancel() {
      if (!state.pending) reset()
    },
    async confirm() {
      const request = state.request
      if (!request || state.pending) return
      state.pending = true
      state.error = ""
      try {
        const error = await request.onConfirm()
        if (state.request !== request) return
        if (error) state.error = error
        else reset()
      } catch (error) {
        if (state.request === request)
          state.error = error instanceof Error
            ? error.message
            : "The action could not be completed. Try again."
      } finally {
        if (state.request === request) state.pending = false
      }
    },
  }
}

export type Confirmation = ReturnType<typeof createConfirmation>
