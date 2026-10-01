import type { FieldTransport } from "./api"
import { fileBrowseFromSearch, type FileBrowse } from "./browse"

export type SessionFile = { id: string; name: string; size?: number; media_type?: string; unavailable?: boolean }
type FilePage = { files: SessionFile[]; next_cursor?: string; previous_cursor?: string }
type Status = "loading" | "ready" | "resolved" | "denied" | "unavailable"

export function createSessionFilePicker(transport: FieldTransport, initialBrowse = fileBrowseFromSearch("")) {
  const state = $state({
    status: "loading" as Status, label: "Select session files", files: [] as SessionFile[],
    capabilities: [] as string[], mediaTypes: [] as string[], selected: [] as SessionFile[],
    browse: initialBrowse, nextCursor: "", previousCursor: "", maxFiles: null as number | null,
    filterName: initialBrowse.name, filterMediaType: initialBrowse.mediaType,
    busy: false, pageLoading: false, saved: false, error: "", opening: "",
  })
  let disposed = false
  let pageGeneration = 0
  let pageAbort: AbortController | null = null
  const allowed = (file: SessionFile) => state.mediaTypes.length === 0 || state.mediaTypes.some((type) =>
    type === file.media_type || type.endsWith("/*") && file.media_type?.startsWith(type.slice(0, -1)))

  async function check(response: Response): Promise<Response> {
    if (!response.ok) {
      if (response.status === 409) state.status = "resolved"
      else if ([401, 403].includes(response.status)) state.status = "denied"
      throw new Error(response.status === 409 ? "This input is already resolved." : `The operation failed (${response.status}).`)
    }
    return response
  }

  async function browse(next: FileBrowse): Promise<void> {
    state.browse = { ...next }
    state.filterName = next.name
    state.filterMediaType = next.mediaType
    if (state.status !== "ready" || !state.capabilities.includes("session.file.list") || disposed) return
    const current = ++pageGeneration
    pageAbort?.abort()
    pageAbort = new AbortController()
    state.pageLoading = true
    state.error = ""
    try {
      const response = await transport.files(next, state.mediaTypes, pageAbort.signal)
      if (disposed || current !== pageGeneration) return
      const page = await (await check(response)).json() as FilePage
      if (disposed || current !== pageGeneration) return
      state.files = page.files
      state.nextCursor = page.next_cursor ?? ""
      state.previousCursor = page.previous_cursor ?? ""
      const metadata = new Map(page.files.map((file) => [file.id, file]))
      state.selected = state.selected.map((file) => metadata.get(file.id) ?? file)
    } catch (error) {
      if (!disposed && current === pageGeneration)
        state.error = error instanceof Error ? error.message : "The files could not be loaded."
    } finally { if (current === pageGeneration) state.pageLoading = false }
  }

  async function load(): Promise<void> {
    if (state.busy || disposed) return
    pageAbort?.abort()
    pageGeneration += 1
    state.status = "loading"
    state.error = ""
    try {
      const field = await (await check(await transport.read())).json() as {
        label: string; inputs: unknown; value?: { file_ids?: unknown; file_id?: unknown }; capabilities: string[]
      }
      if (disposed) return
      state.label = field.label === "Select a session file" ? "Select session files" : field.label
      state.capabilities = field.capabilities
      if (field.inputs === null || typeof field.inputs !== "object" || Array.isArray(field.inputs)) throw new Error("File picker inputs must be a JSON object.")
      const inputs = field.inputs as { media_types?: unknown; max_files?: unknown }
      if (inputs.media_types !== undefined && (!Array.isArray(inputs.media_types) || inputs.media_types.some((type) => typeof type !== "string"))) throw new Error("media_types must be an array of strings.")
      if (inputs.max_files !== undefined && (!Number.isSafeInteger(inputs.max_files) || Number(inputs.max_files) < 1)) throw new Error("max_files must be a positive integer.")
      state.mediaTypes = (inputs.media_types as string[] | undefined) ?? []
      state.maxFiles = inputs.max_files === undefined ? null : Number(inputs.max_files)
      const ids = Array.isArray(field.value?.file_ids) ? field.value.file_ids.filter((id): id is string => typeof id === "string") : typeof field.value?.file_id === "string" ? [field.value.file_id] : []
      state.selected = [...new Set(ids)].map((id) => ({ id, name: id }))
      if (state.capabilities.includes("session.file.list")) {
        // Hydrate just the saved selection, never the whole file collection.
        const available = new Map<string, SessionFile>()
        for (let start = 0; start < state.selected.length; start += 100) {
          const batch = state.selected.slice(start, start + 100).map((file) => file.id)
          const response = await (await check(await transport.references(batch))).json() as FilePage
          if (disposed) return
          for (const file of response.files) available.set(file.id, file)
        }
        state.selected = state.selected.map((file) => available.get(file.id) ?? { ...file, unavailable: true })
      }
      state.saved = false
      state.status = "ready"
      await browse(state.browse)
    } catch (error) {
      if (disposed) return
      state.error = error instanceof Error ? error.message : "The file picker could not be loaded."
      if (state.status === "loading") state.status = "unavailable"
    }
  }

  function remove(id: string): void {
    if (state.busy) return
    state.selected = state.selected.filter((file) => file.id !== id)
    state.saved = false
    state.error = ""
  }

  function select(file: SessionFile): void {
    if (state.busy || state.status !== "ready") return
    if (state.selected.some((entry) => entry.id === file.id)) { remove(file.id); return }
    if (state.maxFiles !== null && state.selected.length >= state.maxFiles) { state.error = `Select no more than ${state.maxFiles} files.`; return }
    state.selected = [...state.selected, file]
    state.saved = false
    state.error = ""
  }

  async function save(): Promise<boolean> {
    if (state.busy || state.status !== "ready") return false
    if (state.selected.some((file) => file.unavailable)) { state.error = "Remove unavailable files before saving."; return false }
    state.busy = true
    state.saved = false
    state.error = ""
    try {
      await check(await transport.save(state.selected.map((file) => file.id)))
      state.saved = true
      return true
    }
    catch (error) { state.error = error instanceof Error ? error.message : "The selection could not be saved."; return false }
    finally { state.busy = false }
  }

  async function upload(files: File[]): Promise<void> {
    if (state.busy || state.status !== "ready" || !state.capabilities.includes("session.file.upload")) return
    if (state.maxFiles !== null && state.selected.length + files.length > state.maxFiles) { state.error = `Select no more than ${state.maxFiles} files.`; return }
    if (files.some((file) => !allowed({ id: "", name: file.name, size: file.size, media_type: file.type }))) { state.error = "A file's media type is not accepted."; return }
    state.busy = true
    state.error = ""
    state.saved = false
    try {
      for (const file of files) {
        const created = await (await check(await transport.createFile(file))).json() as { file: { ref: { id: string } }; upload_url: string }
        await check(await transport.upload(created.upload_url, file))
        const completed = await (await check(await transport.finishFile(created.file.ref.id))).json() as SessionFile
        if (disposed) return
        state.selected = [...state.selected, completed]
      }
    } catch (error) { state.error = error instanceof Error ? error.message : "A file could not be uploaded." }
    finally { state.busy = false }
  }

  async function download(id: string): Promise<string | null> {
    if (state.status !== "ready" || state.opening || !state.capabilities.includes("session.file.read")) return null
    state.opening = id
    state.error = ""
    try { return ((await (await check(await transport.download(id))).json()) as { url: string }).url }
    catch (error) { state.error = error instanceof Error ? error.message : "The file could not be opened."; return null }
    finally { state.opening = "" }
  }

  return { state, load, browse, select, remove, save, upload, download,
    dispose: () => { disposed = true; pageGeneration += 1; pageAbort?.abort() },
  }
}
