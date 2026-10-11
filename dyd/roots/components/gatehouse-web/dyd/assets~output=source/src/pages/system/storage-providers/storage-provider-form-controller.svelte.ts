import {
  systemAdministration,
  type SystemKeychain,
  type SystemStorageProvider,
} from "../../../app/system"
type Form = {
  alias: string
  protocol: string
  endpoint: string
  region: string
  bucket: string
  accessKeyID: string
  keychain: string
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
  keychain: "",
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
  const state = $state({
    provider: null as SystemStorageProvider | null,
    keychains: [] as SystemKeychain[],
    form: blank(),
    error: "",
    loading: false,
    saving: false,
    editing: false,
  })
  async function keychains() {
    const r = await systemAdministration("keychains")
    if (r.status === 401) onAuthenticationLost()
    else if (r.status === 403) onSystemAccessChange("denied")
    else if (r.ok) state.keychains = (await r.json()) as SystemKeychain[]
  }
  async function load(id: string) {
    state.loading = true
    state.error = ""
    try {
      const r = await systemAdministration(
        `storage-providers/${encodeURIComponent(id)}`,
      )
      if (r.status === 401) onAuthenticationLost()
      else if (r.status === 403) onSystemAccessChange("denied")
      else if (r.status === 404) state.error = "Storage provider was not found."
      else if (!r.ok) state.error = "Storage provider could not be loaded."
      else state.provider = (await r.json()) as SystemStorageProvider
    } catch {
      state.error = "Storage provider could not be loaded."
    } finally {
      state.loading = false
    }
  }
  async function beginEdit() {
    if (state.provider === null) return
    state.form = {
      alias: state.provider.alias,
      protocol: state.provider.protocol,
      endpoint: state.provider.endpoint ?? "",
      region: state.provider.region ?? "",
      bucket: state.provider.bucket ?? "",
      accessKeyID: state.provider.access_key_id ?? "",
      keychain: state.provider.keychain?.id ?? "",
      secret: "",
      enabled: state.provider.enabled,
    }
    await keychains()
    state.editing = true
  }
  function valid(replace: boolean) {
    if (state.form.protocol === "s3" && state.form.keychain === "") {
      state.error = "A keychain is required for S3 storage."
      return false
    }
    if (
      replace &&
      state.form.protocol === "s3" &&
      state.form.secret.trim() === ""
    ) {
      state.error =
        "A secret access key is required when changing protocol or keychain."
      return false
    }
    return true
  }
  async function save(
    path: string,
    method: "POST" | "PATCH",
    revision?: number,
  ): Promise<SystemStorageProvider | null> {
    state.saving = true
    try {
      const f = state.form
      const r = await systemAdministration(path, method, {
        alias: f.alias,
        protocol: f.protocol,
        endpoint: f.protocol === "embedded" ? undefined : optional(f.endpoint),
        region: f.protocol === "embedded" ? undefined : optional(f.region),
        bucket: f.protocol === "embedded" ? undefined : optional(f.bucket),
        access_key_id:
          f.protocol === "embedded" ? undefined : optional(f.accessKeyID),
        keychain: f.protocol === "embedded" ? undefined : optional(f.keychain),
        secret_access_key:
          f.protocol === "embedded" ? undefined : optional(f.secret),
        enabled: f.enabled,
        ...(revision === undefined ? {} : { expected_revision: revision }),
      })
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
          await beginEdit()
        }
        state.error =
          "This provider changed elsewhere. Review the reloaded settings before saving."
        return null
      }
      if (!r.ok) {
        state.error = "Storage provider could not be saved."
        return null
      }
      return (await r.json()) as SystemStorageProvider
    } catch {
      state.error = "Storage provider could not be saved."
      return null
    } finally {
      state.saving = false
    }
  }
  async function create() {
    if (state.saving) return null
    state.error = ""
    if (!valid(state.form.protocol === "s3")) return null
    return await save("storage-providers", "POST")
  }
  async function update() {
    if (state.saving) return false
    if (!state.provider) return false
    state.error = ""
    const replace =
      state.provider.protocol !== state.form.protocol ||
      state.provider.keychain?.id !== optional(state.form.keychain)
    if (!valid(replace)) return false
    const saved = await save(
      `storage-providers/${encodeURIComponent(state.provider.id)}`,
      "PATCH",
      state.provider.revision,
    )
    if (!saved) return false
    state.provider = saved
    state.editing = false
    return true
  }
  return { state, keychains, load, beginEdit, create, update }
}
