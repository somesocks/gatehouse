import {
  systemAdministration,
  type SystemAgentModel,
} from "../../../app/system"
import { createLoadState } from "../../../utils/load-state.svelte"
type Form = {
  alias: string
  provider: string
  model: string
  parameters: string
  compaction: string
  maxTurns: number
  maxOutputTokens: number
  enabled: boolean
}
const blank = (): Form => ({
  alias: "",
  provider: "",
  model: "",
  parameters: "{}",
  compaction:
    '{"algorithm":"mcmtr","history_bytes":98304,"buffer_bytes":49152}',
  maxTurns: 127,
  maxOutputTokens: 16000,
  enabled: true,
})
export function createAgentModelFormController({
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
    model: null as SystemAgentModel | null,
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
  async function load(id: string): Promise<void> {
    disposed = false
    if (owner !== id) {
      owner = id
      resource.reset()
      mutation.stop()
      state.model = null
      state.editing = false
      state.form = blank()
    }
    state.error = ""
    const request = resource.begin()
    try {
      const response = await systemAdministration(
        `agent-models/${encodeURIComponent(id)}`,
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
        throw new Error("Agent model was not found.")
      }
      if (!response.ok) {
        throw new Error("Agent model could not be loaded.")
      }
      const model = (await response.json()) as SystemAgentModel
      request.commit(() => {
        state.model = model
      })
    } catch (error) {
      request.fail(error)
    } finally {
      request.finish()
    }
  }
  async function beginEdit(): Promise<void> {
    if (disposed || state.model === null || state.editing) return
    editRevision = state.model.revision
    state.conflict = false
    state.form = {
      alias: state.model.alias,
      provider: state.model.provider,
      model: state.model.model,
      parameters: state.model.parameters,
      compaction: state.model.compaction,
      maxTurns: state.model.max_turns,
      maxOutputTokens: state.model.max_output_tokens,
      enabled: state.model.enabled,
    }
    state.editing = true
  }
  function valid(): boolean {
    if (state.form.provider === "") {
      state.error = "A provider is required."
      return false
    }
    return true
  }
  async function create(): Promise<SystemAgentModel | null> {
    if (disposed || state.saving) return null
    state.error = ""
    if (!valid()) return null
    return await save("agent-models", "POST", state.form)
  }
  async function update(): Promise<boolean> {
    if (disposed || state.saving) return false
    if (state.model === null) return false
    state.error = ""
    if (!valid()) return false
    const saved = await save(
      `agent-models/${encodeURIComponent(state.model.id)}`,
      "PATCH",
      state.form,
      editRevision,
    )
    if (saved === null) return false
    if (disposed) return false
    resource.stop()
    state.model = saved
    state.editing = false
    return true
  }
  async function save(
    path: string,
    method: "POST" | "PATCH",
    form: Form,
    revision?: number,
  ): Promise<SystemAgentModel | null> {
    const request = mutation.begin()
    try {
      const response = await systemAdministration(
        path,
        method,
        {
          alias: form.alias,
          provider: form.provider,
          model: form.model,
          parameters: form.parameters,
          compaction: form.compaction,
          max_turns: form.maxTurns,
          max_output_tokens: form.maxOutputTokens,
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
        if (state.model !== null) {
          await load(state.model.id)
        }
        if (!request.current()) return null
        state.error =
          "This model changed elsewhere. Your draft is preserved; review it before retrying."
        state.conflict = true
        return null
      }
      if (!response.ok) {
        state.error = "Agent model could not be saved."
        return null
      }
      const result = (await response.json()) as SystemAgentModel
      return request.current() ? result : null
    } catch {
      if (request.current()) state.error = "Agent model could not be saved."
      return null
    } finally {
      request.finish()
    }
  }
  function stop() {
    disposed = true
    resource.stop()
    mutation.stop()
    state.editing = false
  }
  function replaceDraft() {
    if (!disposed) {
      state.editing = false
      state.error = ""
      return beginEdit()
    }
  }
  return { state, load, beginEdit, create, update, stop, replaceDraft }
}
