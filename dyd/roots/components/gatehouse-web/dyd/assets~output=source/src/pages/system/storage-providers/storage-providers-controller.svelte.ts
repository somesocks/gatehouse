import type { ActivityClient } from "../../../app/activity"
import { systemAdministration, type SystemStorageProvider } from "../../../app/system"

type StorageProvidersControllerOptions = {
  activity: ActivityClient
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}

export function createStorageProvidersController({ activity, onAuthenticationLost, onSystemAccessChange }: StorageProvidersControllerOptions) {
  const state = $state({
    providers: [] as SystemStorageProvider[],
    error: "",
    saving: false,
    form: { alias: "", protocol: "embedded", endpoint: "", region: "", bucket: "", accessKeyID: "", keychain: "", secretAccessKey: "" },
  })
  let unsubscribe: (() => void) | undefined

  const optional = (value: string): string | undefined => value.trim() === "" ? undefined : value.trim()

  async function load(): Promise<void> {
    state.error = ""
    try {
      const response = await systemAdministration("storage-providers")
      if (response.status === 401) { onAuthenticationLost(); return }
      if (response.status === 403) { onSystemAccessChange("denied"); return }
      if (!response.ok) { state.error = "Storage providers could not be loaded."; return }
      state.providers = await response.json() as SystemStorageProvider[]
    } catch { state.error = "Storage providers could not be loaded." }
  }

  async function save(path: string, method: "POST" | "PATCH", body: Record<string, unknown>): Promise<boolean> {
    state.error = ""
    state.saving = true
    try {
      const response = await systemAdministration(path, method, body)
      if (response.status === 401) { onAuthenticationLost(); return false }
      if (response.status === 403) { onSystemAccessChange("denied"); return false }
      if (response.status === 409) { state.error = "This provider changed elsewhere. The latest settings have been reloaded."; await load(); return false }
      if (!response.ok) { state.error = "Storage provider could not be saved."; return false }
      await load()
      return true
    } catch { state.error = "Storage provider could not be saved."; return false } finally { state.saving = false }
  }

  async function create(): Promise<void> {
    if (!await save("storage-providers", "POST", { alias: state.form.alias, protocol: state.form.protocol, endpoint: optional(state.form.endpoint), region: optional(state.form.region), bucket: optional(state.form.bucket), access_key_id: optional(state.form.accessKeyID), keychain: optional(state.form.keychain), secret_access_key: optional(state.form.secretAccessKey), enabled: true })) return
    state.form = { alias: "", protocol: "embedded", endpoint: "", region: "", bucket: "", accessKeyID: "", keychain: "", secretAccessKey: "" }
  }

  async function setEnabled(provider: SystemStorageProvider, enabled: boolean): Promise<void> {
    await save(`storage-providers/${encodeURIComponent(provider.id)}`, "PATCH", { alias: provider.alias, protocol: provider.protocol, endpoint: provider.endpoint, region: provider.region, bucket: provider.bucket, access_key_id: provider.access_key_id, keychain: provider.keychain?.id, enabled, expected_revision: provider.revision })
  }

  function start(): () => void {
    stop()
    unsubscribe = activity.subscribe([{ name: "storage-providers", topic: "sys", events: ["storage_provider.*"] }], async () => await load())
    return stop
  }

  function stop(): void {
    unsubscribe?.()
    unsubscribe = undefined
  }

  return { state, load, create, setEnabled, start, stop }
}
