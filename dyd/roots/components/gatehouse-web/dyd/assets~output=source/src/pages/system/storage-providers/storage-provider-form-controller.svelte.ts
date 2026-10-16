import {
  systemAdministration,
  type SystemStorageProvider,
} from "../../../app/system"
import { createLoadState } from "../../../utils/load-state.svelte"
type Form = {
  alias: string
  protocol: string
  endpoint: string
  region: string
  bucket: string
  accessKeyID: string
  secret: string
  enabled: boolean
}
const blank = (): Form => ({
  alias: "",
  protocol: "embedded",
  endpoint: "",
  region: "",
  bucket: "",
  accessKeyID: "",
  secret: "",
  enabled: true,
})
const optional = (value: string) =>
  value.trim() === "" ? undefined : value.trim()
export function createStorageProviderFormController({
  onAuthenticationLost,
  onSystemAccessChange,
}: {
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const resource = createLoadState()
  const mutation = createLoadState()
  const errors = $state({ value: "" })
  let owner = ""
  let disposed = false
  let editRevision: number | undefined
  const state = $state({
    provider: null as SystemStorageProvider | null,
    form: blank(),
    get error() {
      return errors.value || resource.state.error
    },
    set error(value: string) {
      errors.value = value
    },
    get loading() {
      return resource.loading
    },
    get reloading() {
      return resource.reloading
    },
    get saving() {
      return mutation.state.pending
    },
    editing: false,
    conflict: false,
  })
  async function load(id: string) {
    disposed = false
    if (owner !== id) {
      owner = id
      resource.reset()
      mutation.stop()
      state.provider = null
      state.editing = false
      state.form = blank()
    }
    const request = resource.begin()
    state.error = ""
    try {
      const r = await systemAdministration(
        `storage-providers/${encodeURIComponent(id)}`,
        "GET",
        undefined,
        request.signal,
      )
      if (!request.current()) return
      if (r.status === 401) onAuthenticationLost()
      else if (r.status === 403) onSystemAccessChange("denied")
      else if (r.status === 404)
        throw new Error("Storage provider was not found.")
      else if (!r.ok) throw new Error("Storage provider could not be loaded.")
      else {
        const provider = (await r.json()) as SystemStorageProvider
        request.commit(() => {
          state.provider = provider
        })
      }
    } catch (error) {
      request.fail(error)
    } finally {
      request.finish()
    }
  }
  async function beginEdit() {
    if (disposed || state.provider === null || state.editing) return
    editRevision = state.provider.revision
    state.conflict = false
    state.form = {
      alias: state.provider.alias,
      protocol: state.provider.protocol,
      endpoint: state.provider.endpoint ?? "",
      region: state.provider.region ?? "",
      bucket: state.provider.bucket ?? "",
      accessKeyID: state.provider.access_key_id ?? "",
      secret: "",
      enabled: state.provider.enabled,
    }
    state.editing = true
  }
  function valid(replace: boolean) {
    if (
      replace &&
      state.form.protocol === "s3" &&
      state.form.secret.trim() === ""
    ) {
      state.error = "A secret access key is required when changing protocol."
      return false
    }
    return true
  }
  async function save(
    path: string,
    method: "POST" | "PATCH",
    revision?: number,
  ): Promise<SystemStorageProvider | null> {
    const request = mutation.begin()
    try {
      const f = state.form
      const r = await systemAdministration(
        path,
        method,
        {
          alias: f.alias,
          protocol: f.protocol,
          endpoint:
            f.protocol === "embedded" ? undefined : optional(f.endpoint),
          region: f.protocol === "embedded" ? undefined : optional(f.region),
          bucket: f.protocol === "embedded" ? undefined : optional(f.bucket),
          access_key_id:
            f.protocol === "embedded" ? undefined : optional(f.accessKeyID),
          secret_access_key:
            f.protocol === "embedded" ? undefined : optional(f.secret),
          enabled: f.enabled,
          ...(revision === undefined ? {} : { expected_revision: revision }),
        },
        request.signal,
      )
      if (!request.current()) return null
      if (r.status === 401) {
        onAuthenticationLost()
        return null
      }
      if (r.status === 403) {
        onSystemAccessChange("denied")
        return null
      }
      if (r.status === 409) {
        if (state.provider) {
          await load(state.provider.id)
        }
        if (!request.current()) return null
        state.error =
          "This provider changed elsewhere. Your draft is preserved; review it before retrying."
        state.conflict = true
        return null
      }
      if (!r.ok) {
        state.error = "Storage provider could not be saved."
        return null
      }
      const result = (await r.json()) as SystemStorageProvider
      return request.current() ? result : null
    } catch {
      if (request.current())
        state.error = "Storage provider could not be saved."
      return null
    } finally {
      request.finish()
    }
  }
  async function create() {
    if (disposed || state.saving) return null
    state.error = ""
    if (!valid(state.form.protocol === "s3")) return null
    return await save("storage-providers", "POST")
  }
  async function update() {
    if (disposed || state.saving) return false
    if (!state.provider) return false
    state.error = ""
    const replace = state.provider.protocol !== state.form.protocol
    if (!valid(replace)) return false
    const saved = await save(
      `storage-providers/${encodeURIComponent(state.provider.id)}`,
      "PATCH",
      editRevision,
    )
    if (!saved) return false
    if (disposed) return false
    resource.stop()
    state.provider = saved
    state.editing = false
    return true
  }
  function stop() {
    disposed = true
    resource.stop()
    mutation.stop()
    state.form.secret = ""
    state.editing = false
  }
  function replaceDraft() {
    if (!disposed) {
      state.editing = false
      state.error = ""
      return beginEdit()
    }
  }
  return {
    state,
    load,
    beginEdit,
    create,
    update,
    stop,
    replaceDraft,
  }
}
