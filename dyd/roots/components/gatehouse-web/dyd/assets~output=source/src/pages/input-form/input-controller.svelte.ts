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

type State = "loading" | "ready" | "resolved" | "denied" | "unavailable" | "submitted" | "cancelled"

export function createInputFormController(transport: InputTransport) {
  const state = $state({
    status: "loading" as State,
    form: null as InputForm | null,
    draft: {} as Record<string, unknown>,
    error: "",
    terminalPending: false,
    needsReload: false,
    invalidAnswer: false,
  })
  let queue = Promise.resolve()
  let generation = 0
  let draftRevision = 0
  const uploads = new Set<Promise<boolean>>()

  function upload(path: string[], files: File[], save: (value: InputFileSummary) => Promise<boolean>): Promise<boolean> {
    if (state.status !== "ready" || state.terminalPending || state.needsReload) return Promise.resolve(false)
    const operation = (async () => {
      try {
        for (const file of files) {
          const created = await transport.createFile(path, file)
          if (!created.ok) throw new Error(`File upload could not start (${created.status}).`)
          const started = (await created.json()) as { file: { ref: { id: string } }; upload_url: string }
          const put = await transport.uploadFile(started.upload_url, file)
          if (!put.ok) throw new Error(`File upload failed (${put.status}).`)
          const finished = await transport.finishFile(path, started.file.ref.id)
          if (!finished.ok) throw new Error(`File upload could not finish (${finished.status}).`)
          const summary = (await finished.json()) as InputFileSummary
          if (!(await save(summary))) return false
        }
        return true
      } catch (error) {
        state.error = error instanceof Error ? error.message : "File upload failed."
        state.invalidAnswer = true
        return false
      }
    })()
    uploads.add(operation)
    return operation.finally(() => uploads.delete(operation))
  }

  function dispose(): void {
    generation += 1
  }

  async function load(): Promise<void> {
    await queue
    const value = ++generation
    state.status = "loading"
    state.error = ""
    state.needsReload = false
    state.invalidAnswer = false
    try {
      const response = await transport.read()
      if (value !== generation) return
      if (response.status === 409) {
        state.status = "resolved"
        return
      }
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        state.status = "denied"
        return
      }
      if (!response.ok) throw new Error("The form could not be loaded. Try again.")
      const document = parseInputDocument(await response.text())
      if (value !== generation) return
      state.form = document.form
      state.draft = document.draft
      state.status = "ready"
    } catch {
      if (value === generation) state.status = "unavailable"
    }
  }

  // A focus refresh must not replace the form with a loading screen or
  // overwrite edits made while the read is in flight.
  async function refresh(): Promise<void> {
    if (state.status !== "ready" || state.terminalPending || state.needsReload || state.invalidAnswer) return
    await queue
    if (state.status !== "ready" || state.terminalPending || state.needsReload || state.invalidAnswer) return
    const value = ++generation
    const revision = draftRevision
    try {
      const response = await transport.read()
      if (value !== generation || revision !== draftRevision || state.status !== "ready" || state.terminalPending) return
      if (response.status === 409) {
        state.status = "resolved"
        return
      }
      if (response.status === 401 || response.status === 403 || response.status === 404) {
        state.status = "denied"
        return
      }
      if (!response.ok) throw new Error("The shared draft could not be refreshed.")
      const document = parseInputDocument(await response.text())
      if (value !== generation || revision !== draftRevision || state.status !== "ready" || state.terminalPending) return
      if (JSON.stringify(state.draft) !== JSON.stringify(document.draft)) {
        state.draft = document.draft
        state.error = ""
      }
    } catch {
      if (value === generation && revision === draftRevision && state.status === "ready")
        state.error = "The shared draft could not be refreshed. Try again when you return."
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

  function enqueue(path: string[], value?: unknown, remove = false): Promise<boolean> {
    if (state.status !== "ready" || state.terminalPending && uploads.size === 0 || state.needsReload) return Promise.resolve(false)
    let body: string
    try {
      body = remove
        ? JSON.stringify({ op: "remove", path })
        : `{"op":"set","path":${JSON.stringify(path)},"value":${encodeFieldValue(descriptorAt(path), value)}}`
    } catch (error) {
      state.error = error instanceof Error ? error.message : "Invalid answer."
      state.invalidAnswer = true
      return Promise.resolve(false)
    }
    state.draft = patchDraft(state.draft, path, value, remove)
    draftRevision += 1
    state.error = ""
    state.invalidAnswer = false
    let success = false
    const operation = queue.then(async () => {
      try {
        if (state.needsReload) return
        const response = await transport.patch(body)
        if (response.status === 409) {
          state.status = "resolved"
          return
        }
        if (response.status === 401 || response.status === 403) {
          state.status = "denied"
          return
        }
        if (!response.ok) throw new Error(response.status === 422 ? (await response.text()).trim() : "Your changes could not be saved. Reload to retry.")
        success = true
      } catch (error) {
        state.error = error instanceof Error ? error.message : "Your changes could not be saved."
        state.needsReload = true
      }
    })
    queue = operation.then(() => {})
    // A failed write may have left subsequent optimistic edits unsaved. Reload
    // the authoritative shared draft before submitting or making further edits.
    return operation.then(() => success)
  }

  async function finish(action: "submit" | "cancel"): Promise<void> {
    if (state.status !== "ready" || state.terminalPending) return
    state.terminalPending = true
    state.error = ""
    try {
      if (action === "submit") await Promise.all([...uploads])
      await queue
      if (state.status !== "ready") return
      if (action === "submit") {
        if (state.needsReload || state.invalidAnswer) {
          state.error = "Reload the form before submitting unsaved changes."
          return
        }
        for (const update of requiredBooleanDefaults(state.form!, state.draft)) {
          if (!(await enqueueWhileFinishing(update.path, update.value))) return
        }
      }
      const response = await transport.terminal(action)
      if (response.status === 409) {
        state.status = "resolved"
        return
      }
      if (response.status === 401 || response.status === 403) {
        state.status = "denied"
        return
      }
      if (response.status === 422) {
        state.error = (await response.text()).trim()
        return
      }
      if (!response.ok) throw new Error("The form could not be submitted. Try again.")
      state.status = action === "submit" ? "submitted" : "cancelled"
    } catch (error) {
      state.error = error instanceof Error ? error.message : "The request failed. Try again."
    } finally {
      state.terminalPending = false
    }
  }

  async function enqueueWhileFinishing(path: string[], value: unknown): Promise<boolean> {
    const body = `{"op":"set","path":${JSON.stringify(path)},"value":${encodeFieldValue(descriptorAt(path), value)}}`
    try {
      const response = await transport.patch(body)
      if (response.status === 409) {
        state.status = "resolved"
        return false
      }
      if (response.status === 401 || response.status === 403) {
        state.status = "denied"
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
      state.error = "A checkbox could not be saved. Reload the form to retry."
      state.needsReload = true
      return false
    }
  }

  return {
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
