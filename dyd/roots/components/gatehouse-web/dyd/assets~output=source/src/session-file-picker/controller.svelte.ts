import type { FieldTransport } from "./api"
import { fileBrowseFromSearch, type FileBrowse } from "./browse"
import { createLoadState } from "../utils/load-state.svelte"

export type SessionFile = {
  id: string
  name: string
  size?: number
  media_type?: string
  unavailable?: boolean
}
type FilePage = {
  files: SessionFile[]
  next_cursor?: string
  previous_cursor?: string
}
type Status = "loading" | "ready" | "resolved" | "denied" | "unavailable"

export function createSessionFilePicker(
  transport: FieldTransport,
  initialBrowse = fileBrowseFromSearch(""),
) {
  const resource = createLoadState()
  const pageLoad = createLoadState()
  const mutation = createLoadState()
  const downloadLoad = createLoadState()
  const lifetime = new AbortController()
  let dirty = false
  let selectionRevision = 0
  let requestedBrowse = initialBrowse
  const state = $state({
    status: "loading" as Status,
    label: "Select session files",
    files: [] as SessionFile[],
    capabilities: [] as string[],
    mediaTypes: [] as string[],
    selected: [] as SessionFile[],
    browse: initialBrowse,
    nextCursor: "",
    previousCursor: "",
    maxFiles: null as number | null,
    filterName: initialBrowse.name,
    filterMediaType: initialBrowse.mediaType,
    get busy() {
      return mutation.state.pending
    },
    get pageLoading() {
      return pageLoad.state.pending
    },
    get filesLoaded() {
      return pageLoad.state.hasLoaded
    },
    get reloading() {
      return resource.reloading
    },
    saved: false,
    error: "",
    opening: "",
  })
  let disposed = false
  const allowed = (file: SessionFile) =>
    state.mediaTypes.length === 0 ||
    state.mediaTypes.some(
      (type) =>
        type === file.media_type ||
        (type.endsWith("/*") && file.media_type?.startsWith(type.slice(0, -1))),
    )

  async function check(
    response: Response,
    signal = lifetime.signal,
    scoped = true,
  ): Promise<Response> {
    signal.throwIfAborted()
    if (!response.ok) {
      if (
        scoped &&
        (response.status === 409 || [401, 403].includes(response.status))
      ) {
        state.status = response.status === 409 ? "resolved" : "denied"
        dispose()
      }
      throw new Error(
        response.status === 409
          ? "This input is already resolved."
          : `The operation failed (${response.status}).`,
      )
    }
    return response
  }

  async function browse(next: FileBrowse): Promise<void> {
    if (disposed) return
    requestedBrowse = next
    state.filterName = next.name
    state.filterMediaType = next.mediaType
    if (
      state.status !== "ready" ||
      !state.capabilities.includes("session.file.list") ||
      disposed
    )
      return
    const request = pageLoad.begin(lifetime.signal)
    state.error = ""
    try {
      const response = await transport.files(
        next,
        state.mediaTypes,
        request.signal,
      )
      if (!request.current()) return
      const page = (await (
        await check(response, request.signal)
      ).json()) as FilePage
      if (!request.current()) return
      request.commit(() => {
        state.browse = { ...next }
        state.files = page.files
        state.nextCursor = page.next_cursor ?? ""
        state.previousCursor = page.previous_cursor ?? ""
        const metadata = new Map(page.files.map((file) => [file.id, file]))
        state.selected = state.selected.map(
          (file) => metadata.get(file.id) ?? file,
        )
      })
    } catch (error) {
      request.fail(error)
      if (!disposed && request.current())
        state.error =
          error instanceof Error
            ? error.message
            : "The files could not be loaded."
    } finally {
      request.finish()
    }
  }

  async function load(): Promise<void> {
    if (state.busy || disposed) return
    const request = resource.begin(lifetime.signal)
    const loaded = resource.state.hasLoaded
    const revision = selectionRevision
    pageLoad.stop()
    if (!loaded) state.status = "loading"
    state.error = ""
    try {
      const field = (await (
        await check(await transport.read(request.signal), request.signal)
      ).json()) as {
        label: string
        inputs: unknown
        value?: { file_ids?: unknown; file_id?: unknown }
        capabilities: string[]
      }
      if (!request.current()) return
      if (
        field.inputs === null ||
        typeof field.inputs !== "object" ||
        Array.isArray(field.inputs)
      )
        throw new Error("File picker inputs must be a JSON object.")
      const inputs = field.inputs as {
        media_types?: unknown
        max_files?: unknown
      }
      if (
        inputs.media_types !== undefined &&
        (!Array.isArray(inputs.media_types) ||
          inputs.media_types.some((type) => typeof type !== "string"))
      )
        throw new Error("media_types must be an array of strings.")
      if (
        inputs.max_files !== undefined &&
        (!Number.isSafeInteger(inputs.max_files) ||
          Number(inputs.max_files) < 1)
      )
        throw new Error("max_files must be a positive integer.")
      const ids = Array.isArray(field.value?.file_ids)
        ? field.value.file_ids.filter(
            (id): id is string => typeof id === "string",
          )
        : typeof field.value?.file_id === "string"
          ? [field.value.file_id]
          : []
      let selected = dirty
        ? [...state.selected]
        : [...new Set(ids)].map((id) => ({ id, name: "Selected file" }))
      if (field.capabilities.includes("session.file.list")) {
        // Hydrate just the saved selection, never the whole file collection.
        const available = new Map<string, SessionFile>()
        for (let start = 0; start < selected.length; start += 100) {
          const batch = selected
            .slice(start, start + 100)
            .map((file) => file.id)
          const response = (await (
            await check(
              await transport.references(batch, request.signal),
              request.signal,
            )
          ).json()) as FilePage
          if (!request.current()) return
          for (const file of response.files) available.set(file.id, file)
        }
        selected = selected.map(
          (file) =>
            available.get(file.id) ?? {
              ...file,
              name: "Unavailable file",
              unavailable: true,
            },
        )
      }
      if (
        !request.commit(() => {
          state.label =
            field.label === "Select a session file"
              ? "Select session files"
              : field.label
          state.capabilities = field.capabilities
          state.mediaTypes = (inputs.media_types as string[] | undefined) ?? []
          state.maxFiles =
            inputs.max_files === undefined ? null : Number(inputs.max_files)
          if (revision === selectionRevision) state.selected = selected
          state.status = "ready"
        })
      )
        return
      await browse(requestedBrowse)
    } catch (error) {
      if (!request.current()) return
      state.error =
        error instanceof Error
          ? error.message
          : "The file picker could not be loaded."
      if (state.status === "loading") state.status = "unavailable"
    } finally {
      request.finish()
    }
  }

  function remove(id: string): void {
    if (disposed || state.busy) return
    state.selected = state.selected.filter((file) => file.id !== id)
    dirty = true
    selectionRevision++
    state.saved = false
    state.error = ""
  }

  function select(file: SessionFile): void {
    if (disposed || state.busy || state.status !== "ready") return
    if (state.selected.some((entry) => entry.id === file.id)) {
      remove(file.id)
      return
    }
    if (state.maxFiles !== null && state.selected.length >= state.maxFiles) {
      state.error = `Select no more than ${state.maxFiles} files.`
      return
    }
    state.selected = [...state.selected, file]
    dirty = true
    selectionRevision++
    state.saved = false
    state.error = ""
  }

  async function save(): Promise<boolean> {
    if (disposed || state.busy || state.status !== "ready") return false
    if (state.selected.some((file) => file.unavailable)) {
      state.error = "Remove unavailable files before saving."
      return false
    }
    const request = mutation.begin(lifetime.signal)
    state.saved = false
    state.error = ""
    try {
      await check(
        await transport.save(
          state.selected.map((file) => file.id),
          request.signal,
        ),
        request.signal,
      )
      if (!request.current()) return false
      state.saved = true
      dirty = false
      selectionRevision++
      return true
    } catch (error) {
      if (request.current())
        state.error =
          error instanceof Error
            ? error.message
            : "The selection could not be saved."
      return false
    } finally {
      request.finish()
    }
  }

  async function upload(files: File[]): Promise<void> {
    if (disposed) return
    if (
      state.busy ||
      state.status !== "ready" ||
      !state.capabilities.includes("session.file.upload")
    )
      return
    if (
      state.maxFiles !== null &&
      state.selected.length + files.length > state.maxFiles
    ) {
      state.error = `Select no more than ${state.maxFiles} files.`
      return
    }
    if (
      files.some(
        (file) =>
          !allowed({
            id: "",
            name: file.name,
            size: file.size,
            media_type: file.type,
          }),
      )
    ) {
      state.error = "A file's media type is not accepted."
      return
    }
    const request = mutation.begin(lifetime.signal)
    state.error = ""
    state.saved = false
    try {
      for (const file of files) {
        const created = (await (
          await check(
            await transport.createFile(file, request.signal),
            request.signal,
          )
        ).json()) as { file: { ref: { id: string } }; upload_url: string }
        if (!request.current()) return
        await check(
          await transport.upload(created.upload_url, file, request.signal),
          request.signal,
          false,
        )
        const completed = (await (
          await check(
            await transport.finishFile(created.file.ref.id, request.signal),
            request.signal,
          )
        ).json()) as SessionFile
        if (!request.current()) return
        state.selected = [...state.selected, completed]
        dirty = true
        selectionRevision++
      }
    } catch (error) {
      if (request.current())
        state.error =
          error instanceof Error
            ? error.message
            : "A file could not be uploaded."
    } finally {
      request.finish()
    }
  }

  async function download(id: string): Promise<string | null> {
    if (disposed) return null
    if (
      state.status !== "ready" ||
      state.opening ||
      !state.capabilities.includes("session.file.read")
    )
      return null
    state.opening = id
    const request = downloadLoad.begin(lifetime.signal)
    state.error = ""
    try {
      const result = (await (
        await check(
          await transport.download(id, request.signal),
          request.signal,
        )
      ).json()) as { url: string }
      return request.current() ? result.url : null
    } catch (error) {
      if (request.current())
        state.error =
          error instanceof Error
            ? error.message
            : "The file could not be opened."
      return null
    } finally {
      if (request.current()) state.opening = ""
      request.finish()
    }
  }

  function dispose() {
    disposed = true
    lifetime.abort()
    resource.stop()
    pageLoad.stop()
    mutation.stop()
    downloadLoad.stop()
  }
  return {
    state,
    load,
    browse,
    select,
    remove,
    save,
    upload,
    download,
    dispose,
  }
}
