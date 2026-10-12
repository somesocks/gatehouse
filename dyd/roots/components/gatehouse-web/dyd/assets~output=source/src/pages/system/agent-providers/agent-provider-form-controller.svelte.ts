import {
  systemAdministration,
  type SystemAgentProvider,
  type SystemKeychain,
} from "../../../app/system"
import { createLoadState } from "../../../utils/load-state.svelte"

type Form = {
  alias: string
  protocol: string
  baseURL: string
  keychain: string
  apiKey: string
  enabled: boolean
}
const blank = (): Form => ({
  alias: "",
  protocol: "openai-responses",
  baseURL: "",
  keychain: "",
  apiKey: "",
  enabled: true,
})
const optional = (value: string): string | undefined =>
  value.trim() === "" ? undefined : value.trim()
export function createAgentProviderFormController({
  onAuthenticationLost,
  onSystemAccessChange,
}: {
  onAuthenticationLost: () => void
  onSystemAccessChange: (next: "available" | "denied" | "unavailable") => void
}) {
  const resource = createLoadState()
  const options = createLoadState()
  const mutation = createLoadState()
  const errors = $state({ value: "" })
  let owner = ""
  let disposed = false
  let editRevision: number | undefined
  const state = $state({
    provider: null as SystemAgentProvider | null,
    keychains: [] as SystemKeychain[],
    get keychainsLoading() {
      return options.state.pending
    },
    get keychainsError() {
      return options.state.error
    },
    form: blank(),
    get error() {
      return errors.value || resource.state.error || options.state.error
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
  async function keychains(): Promise<void> {
    if (disposed) return
    const request = options.begin()
    try {
      const response = await systemAdministration(
        "keychains",
        "GET",
        undefined,
        request.signal,
      )
      if (!request.current()) return
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return
      }
      if (!response.ok) throw new Error("Keychains could not be loaded.")
      const keys = (await response.json()) as SystemKeychain[]
      request.commit(() => {
        state.keychains = keys
      })
    } catch (error) {
      request.fail(error)
    } finally {
      request.finish()
    }
  }
  async function load(id: string): Promise<void> {
    disposed = false
    if (owner !== id) {
      owner = id
      resource.reset()
      mutation.stop()
      options.stop()
      state.provider = null
      state.editing = false
      state.form = blank()
    }
    state.error = ""
    const request = resource.begin()
    try {
      const response = await systemAdministration(
        `agent-providers/${encodeURIComponent(id)}`,
        "GET",
        undefined,
        request.signal,
      )
      if (!request.current()) return
      if (response.status === 401) {
        onAuthenticationLost()
        return
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return
      }
      if (response.status === 404) {
        throw new Error("Agent provider was not found.")
      }
      if (!response.ok) {
        throw new Error("Agent provider could not be loaded.")
      }
      const provider = (await response.json()) as SystemAgentProvider
      request.commit(() => {
        state.provider = provider
      })
    } catch (error) {
      request.fail(error)
    } finally {
      request.finish()
    }
  }
  async function beginEdit(): Promise<void> {
    if (disposed || state.provider === null || state.editing) return
    editRevision = state.provider.revision
    state.conflict = false
    state.form = {
      alias: state.provider.alias,
      protocol: state.provider.protocol,
      baseURL: state.provider.base_url ?? "",
      keychain: state.provider.keychain?.id ?? "",
      apiKey: "",
      enabled: state.provider.enabled,
    }
    state.editing = true
    await keychains()
  }
  function validate(replacementRequired: boolean): boolean {
    if (state.form.protocol !== "builtin" && state.form.keychain === "") {
      state.error = "A keychain is required for this provider."
      return false
    }
    if (
      replacementRequired &&
      state.form.protocol !== "builtin" &&
      state.form.apiKey.trim() === ""
    ) {
      state.error = "An API key is required when changing protocol or keychain."
      return false
    }
    return true
  }
  async function create(): Promise<SystemAgentProvider | null> {
    if (disposed || state.saving) return null
    state.error = ""
    if (!validate(state.form.protocol !== "builtin")) return null
    return await save("agent-providers", "POST", state.form)
  }
  async function update(): Promise<boolean> {
    if (disposed || state.saving) return false
    if (state.provider === null) return false
    state.error = ""
    const replacement =
      state.provider.protocol !== state.form.protocol ||
      state.provider.keychain?.id !== optional(state.form.keychain)
    if (!validate(replacement)) return false
    const saved = await save(
      `agent-providers/${encodeURIComponent(state.provider.id)}`,
      "PATCH",
      state.form,
      editRevision,
    )
    if (saved === null) return false
    if (disposed) return false
    resource.stop()
    state.provider = saved
    state.editing = false
    return true
  }
  async function save(
    path: string,
    method: "POST" | "PATCH",
    form: Form,
    revision?: number,
  ): Promise<SystemAgentProvider | null> {
    const request = mutation.begin()
    try {
      const response = await systemAdministration(
        path,
        method,
        {
          alias: form.alias,
          protocol: form.protocol,
          base_url:
            form.protocol === "builtin" ? undefined : optional(form.baseURL),
          keychain:
            form.protocol === "builtin" ? undefined : optional(form.keychain),
          api_key:
            form.protocol === "builtin" ? undefined : optional(form.apiKey),
          enabled: form.enabled,
          ...(revision === undefined ? {} : { expected_revision: revision }),
        },
        request.signal,
      )
      if (!request.current()) return null
      if (response.status === 401) {
        onAuthenticationLost()
        return null
      }
      if (response.status === 403) {
        onSystemAccessChange("denied")
        return null
      }
      if (response.status === 409) {
        if (state.provider !== null) {
          await load(state.provider.id)
        }
        if (!request.current()) return null
        state.error =
          "This provider changed elsewhere. Your draft is preserved; review it before retrying."
        state.conflict = true
        return null
      }
      if (!response.ok) {
        state.error = "Agent provider could not be saved."
        return null
      }
      const result = (await response.json()) as SystemAgentProvider
      return request.current() ? result : null
    } catch {
      if (request.current()) state.error = "Agent provider could not be saved."
      return null
    } finally {
      request.finish()
    }
  }
  function stop() {
    disposed = true
    resource.stop()
    options.stop()
    mutation.stop()
    state.form.apiKey = ""
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
    keychains,
    load,
    beginEdit,
    create,
    update,
    stop,
    replaceDraft,
  }
}
