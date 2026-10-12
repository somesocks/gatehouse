import {
  encodeFieldValue,
  fieldValue,
  parseInputDocument,
  patchDraft,
  requiredBooleanDefaults,
  type InputField,
  type InputForm,
  type InputFileSummary,
} from "./form-data"
import type { InputTransport } from "./input-api"

type State =
  | "loading"
  | "ready"
  | "resolved"
  | "denied"
  | "unavailable"
  | "submitted"
  | "cancelled"

export function createInputFormController(transport: InputTransport) {
  const state = $state({
    status: "loading" as State,
    form: null as InputForm | null,
    draft: {} as Record<string, unknown>,
    error: "",
    terminalPending: false,
    needsReload: false,
    invalidAnswer: false,
    reloading: false,
  })
  let queue = Promise.resolve()
  let generation = 0
  let draftRevision = 0
  let disposed = false
  const lifetime = new AbortController()
  const uploads = new Set<Promise<boolean>>()
  let readRequest: AbortController | undefined
  const unsaved = new Map<
    string,
    { path: string[]; value?: unknown; remove: boolean; body: string }
  >()
  function revoke(status: "denied" | "resolved") {
    state.status = status
    dispose()
  }
  function ended(response: Response) {
    if (response.status === 409) {
      revoke("resolved")
      return true
    }
    if (response.status === 401 || response.status === 403) {
      revoke("denied")
      return true
    }
    return false
  }

  function upload(
    path: string[],
    files: File[],
    save: (value: InputFileSummary) => Promise<boolean>,
  ): Promise<boolean> {
    if (
      disposed ||
      state.status !== "ready" ||
      state.terminalPending ||
      state.needsReload
    )
      return Promise.resolve(false)
    const operation = (async () => {
      try {
        for (const file of files) {
          if (disposed) return false
          const created = await transport.createFile(
            path,
            file,
            lifetime.signal,
          )
          if (disposed) return false
          if (ended(created)) return false
          if (!created.ok)
            throw new Error(`File upload could not start (${created.status}).`)
          const started = (await created.json()) as {
            file: { ref: { id: string } }
            upload_url: string
          }
          if (disposed) return false
          const put = await transport.uploadFile(
            started.upload_url,
            file,
            lifetime.signal,
          )
          if (disposed) return false
          if (!put.ok) throw new Error(`File upload failed (${put.status}).`)
          const finished = await transport.finishFile(
            path,
            started.file.ref.id,
            lifetime.signal,
          )
          if (disposed) return false
          if (ended(finished)) return false
          if (!finished.ok)
            throw new Error(
              `File upload could not finish (${finished.status}).`,
            )
          const summary = (await finished.json()) as InputFileSummary
          if (disposed) return false
          if (!(await save(summary))) return false
        }
        return true
      } catch (error) {
        if (disposed) return false
        state.error =
          error instanceof Error ? error.message : "File upload failed."
        state.invalidAnswer = true
        return false
      }
    })()
    uploads.add(operation)
    return operation.finally(() => uploads.delete(operation))
  }

  function dispose(): void {
    disposed = true
    lifetime.abort()
    readRequest?.abort()
    state.reloading = false
    state.terminalPending = false
    generation += 1
  }

  async function openCustom(path: string[]): Promise<string> {
    await queue
    if (
      disposed ||
      state.status !== "ready" ||
      state.terminalPending ||
      state.needsReload
    )
      throw new Error("The form is not ready.")
    const response = await transport.openCustom(path, lifetime.signal)
    if (disposed) throw new Error("The form was closed.")
    if (ended(response)) throw new Error("The form is no longer available.")
    if (!response.ok)
      throw new Error(`The tool could not be opened (${response.status}).`)
    const launch = (await response.json()) as { url: string }
    if (disposed) throw new Error("The form was closed.")
    return launch.url
  }

  async function load(): Promise<void> {
    if (disposed || state.terminalPending || uploads.size > 0) return
    const value = ++generation
    readRequest?.abort()
    const request = new AbortController()
    readRequest = request
    const signal = AbortSignal.any([lifetime.signal, request.signal])
    await queue
    if (disposed || value !== generation) return
    const loaded = state.form !== null
    const revision = draftRevision
    if (!loaded) state.status = "loading"
    state.reloading = loaded
    state.error = ""
    try {
      const response = await transport.read(signal)
      if (value !== generation) return
      if (response.status === 409) {
        revoke("resolved")
        return
      }
      if (
        response.status === 401 ||
        response.status === 403 ||
        response.status === 404
      ) {
        revoke("denied")
        return
      }
      if (!response.ok)
        throw new Error("The form could not be loaded. Try again.")
      const document = parseInputDocument(await response.text())
      await queue
      if (
        value !== generation ||
        disposed ||
        signal.aborted ||
        state.terminalPending
      )
        return
      state.form = document.form
      if (!loaded || revision === draftRevision) {
        state.draft = [...unsaved.values()].reduce(
          (draft, edit) =>
            patchDraft(draft, edit.path, edit.value, edit.remove),
          document.draft,
        )
      }
      state.status = "ready"
      if (loaded && unsaved.size > 0) {
        state.needsReload = true
        for (const [key, edit] of [...unsaved]) {
          descriptorAt(edit.path)
          const response = await transport.patch(edit.body, signal)
          if (value !== generation || disposed || signal.aborted) return
          if (response.status === 409) {
            revoke("resolved")
            return
          }
          if ([401, 403, 404].includes(response.status)) {
            revoke("denied")
            return
          }
          if (response.status === 422) {
            const error = (await response.text()).trim()
            if (value !== generation || disposed || signal.aborted) return
            state.error = error || "Correct the invalid answer before retrying."
            state.needsReload = false
            state.invalidAnswer = true
            return
          }
          if (!response.ok)
            throw new Error(
              "Your draft is preserved, but the unsaved changes could not be retried.",
            )
          if (unsaved.get(key) === edit) unsaved.delete(key)
        }
      }
      state.needsReload = unsaved.size > 0
      state.invalidAnswer = false
    } catch {
      if (value === generation && !disposed) {
        if (!loaded) state.status = "unavailable"
        state.error = loaded
          ? "The shared draft could not be refreshed. Your changes are preserved; try again."
          : "The form could not be loaded. Try again."
      }
    } finally {
      if (value === generation) state.reloading = false
    }
  }

  // A focus refresh must not replace the form with a loading screen or
  // overwrite edits made while the read is in flight.
  async function refresh(): Promise<void> {
    if (
      disposed ||
      state.status !== "ready" ||
      state.terminalPending ||
      state.needsReload ||
      state.invalidAnswer
    )
      return
    await queue
    if (
      disposed ||
      state.status !== "ready" ||
      state.terminalPending ||
      state.needsReload ||
      state.invalidAnswer
    )
      return
    const value = ++generation
    readRequest?.abort()
    const request = new AbortController()
    readRequest = request
    const signal = AbortSignal.any([lifetime.signal, request.signal])
    const revision = draftRevision
    state.reloading = true
    try {
      const response = await transport.read(signal)
      if (
        value !== generation ||
        disposed ||
        state.status !== "ready" ||
        state.terminalPending
      )
        return
      if (response.status === 409) {
        revoke("resolved")
        return
      }
      if (
        response.status === 401 ||
        response.status === 403 ||
        response.status === 404
      ) {
        revoke("denied")
        return
      }
      if (!response.ok)
        throw new Error("The shared draft could not be refreshed.")
      const document = parseInputDocument(await response.text())
      if (
        value !== generation ||
        disposed ||
        revision !== draftRevision ||
        state.status !== "ready" ||
        state.terminalPending
      )
        return
      if (JSON.stringify(state.draft) !== JSON.stringify(document.draft)) {
        state.draft = document.draft
        state.error = ""
      }
    } catch {
      if (
        value === generation &&
        revision === draftRevision &&
        state.status === "ready"
      )
        state.error =
          "The shared draft could not be refreshed. Try again when you return."
    } finally {
      if (value === generation) state.reloading = false
    }
  }

  function descriptorAt(path: string[]): InputField {
    let fields = state.form?.fields
    let current: InputField | undefined
    for (const name of path) {
      current = fields?.find((field) => field.id === name)
      fields = current?.type === "object" ? current.fields : undefined
    }
    if (current === undefined) throw new Error("Unknown form field")
    return current
  }

  function enqueue(
    path: string[],
    value?: unknown,
    remove = false,
  ): Promise<boolean> {
    if (
      disposed ||
      state.status !== "ready" ||
      (state.terminalPending && uploads.size === 0) ||
      state.needsReload
    )
      return Promise.resolve(false)
    let body: string
    try {
      const field = descriptorAt(path)
      body = remove
        ? JSON.stringify({ op: "remove", path })
        : `{"op":"set","path":${JSON.stringify(path)},"value":${encodeFieldValue(field, value)}}`
    } catch (error) {
      state.error = error instanceof Error ? error.message : "Invalid answer."
      state.invalidAnswer = true
      return Promise.resolve(false)
    }
    state.draft = patchDraft(state.draft, path, value, remove)
    const key = JSON.stringify(path)
    const edit = { path: [...path], value, remove, body }
    unsaved.delete(key)
    unsaved.set(key, edit)
    draftRevision += 1
    state.error = ""
    state.invalidAnswer = false
    let success = false
    const operation = queue.then(async () => {
      try {
        if (disposed || state.needsReload) return
        const response = await transport.patch(body, lifetime.signal)
        if (disposed) return
        if (response.status === 409) {
          revoke("resolved")
          return
        }
        if (response.status === 401 || response.status === 403) {
          revoke("denied")
          return
        }
        if (!response.ok)
          throw new Error(
            response.status === 422
              ? (await response.text()).trim()
              : "Your changes could not be saved. Reload to retry.",
          )
        success = true
        if (unsaved.get(key) === edit) unsaved.delete(key)
      } catch (error) {
        if (disposed) return
        state.error =
          error instanceof Error
            ? error.message
            : "Your changes could not be saved."
        state.needsReload = true
      }
    })
    queue = operation.then(() => {})
    // A failed write may have left subsequent optimistic edits unsaved. Reload
    // the authoritative shared draft before submitting or making further edits.
    return operation.then(() => success)
  }

  async function finish(action: "submit" | "cancel"): Promise<void> {
    if (disposed || state.status !== "ready" || state.terminalPending) return
    state.terminalPending = true
    generation++
    readRequest?.abort()
    state.reloading = false
    state.error = ""
    try {
      if (action === "submit") await Promise.all([...uploads])
      await queue
      if (disposed || state.status !== "ready") return
      if (action === "submit") {
        if (state.needsReload || state.invalidAnswer || unsaved.size > 0) {
          if (!state.invalidAnswer) state.needsReload = true
          state.error = "Reload the form before submitting unsaved changes."
          return
        }
        for (const update of requiredBooleanDefaults(
          state.form!,
          state.draft,
        )) {
          if (!(await enqueueWhileFinishing(update.path, update.value))) return
        }
      }
      if (disposed) return
      const response = await transport.terminal(action, lifetime.signal)
      if (disposed) return
      if (response.status === 409) {
        revoke("resolved")
        return
      }
      if (response.status === 401 || response.status === 403) {
        revoke("denied")
        return
      }
      if (response.status === 422) {
        const error = (await response.text()).trim()
        if (!disposed) state.error = error
        return
      }
      if (!response.ok)
        throw new Error("The form could not be submitted. Try again.")
      state.status = action === "submit" ? "submitted" : "cancelled"
      dispose()
    } catch (error) {
      if (disposed) return
      state.error =
        error instanceof Error
          ? error.message
          : "The request failed. Try again."
    } finally {
      if (!disposed) state.terminalPending = false
    }
  }

  async function enqueueWhileFinishing(
    path: string[],
    value: unknown,
  ): Promise<boolean> {
    if (disposed) return false
    const body = `{"op":"set","path":${JSON.stringify(path)},"value":${encodeFieldValue(descriptorAt(path), value)}}`
    try {
      const response = await transport.patch(body, lifetime.signal)
      if (disposed) return false
      if (response.status === 409) {
        revoke("resolved")
        return false
      }
      if (response.status === 401 || response.status === 403) {
        revoke("denied")
        return false
      }
      if (!response.ok) {
        state.error = "A checkbox could not be saved. Reload the form to retry."
        state.needsReload = true
        return false
      }
      state.draft = patchDraft(state.draft, path, value)
      draftRevision += 1
      return true
    } catch {
      if (disposed) return false
      state.error = "A checkbox could not be saved. Reload the form to retry."
      state.needsReload = true
      return false
    }
  }

  return {
    openCustom,
    state,
    load,
    refresh,
    dispose,
    value: (path: string[]) => fieldValue(state.draft, path),
    set: (path: string[], value: unknown) => enqueue(path, value),
    remove: (path: string[]) => enqueue(path, undefined, true),
    upload,
    submit: () => finish("submit"),
    cancel: () => finish("cancel"),
  }
}
