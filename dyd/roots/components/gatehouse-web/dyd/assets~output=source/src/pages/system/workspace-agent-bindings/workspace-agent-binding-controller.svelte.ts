import {
  systemAdministration,
  type SystemWorkspaceAgent,
} from "../../../app/system"
import { createLoadState } from "../../../utils/load-state.svelte"

const optional = (value: string) =>
  value.trim() === "" ? undefined : value.trim()

const rateLimits = (value: string) =>
  value.trim() === "" ? undefined : (JSON.parse(value) as unknown)

export function createWorkspaceAgentBindingController({
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
    binding: null as SystemWorkspaceAgent | null,
    form: {
      workspace: "",
      alias: "",
      model: "",
      label: "",
      systemPrompt: "",
      prelude: "",
      rateLimits: "",
      default: false,
      enabled: true,
    },
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

  async function load(workspace: string, bindingID: string) {
    disposed = false
    const key = `${workspace}/${bindingID}`
    if (owner !== key) {
      owner = key
      resource.reset()
      mutation.stop()
      state.binding = null
      state.editing = false
    }
    const request = resource.begin()
    state.error = ""
    try {
      const response = await systemAdministration(
        `workspace-agents/${encodeURIComponent(workspace)}/${encodeURIComponent(bindingID)}`,
        "GET",
        undefined,
        request.signal,
      )
      if (!request.current()) return
      if (response.status === 401) onAuthenticationLost()
      else if (response.status === 403) onSystemAccessChange("denied")
      else if (response.status === 404)
        throw new Error("Workspace agent binding was not found.")
      else if (!response.ok)
        throw new Error("Workspace agent binding could not be loaded.")
      else {
        const binding = (await response.json()) as SystemWorkspaceAgent
        request.commit(() => {
          state.binding = binding
        })
      }
    } catch (error) {
      request.fail(error)
    } finally {
      request.finish()
    }
  }

  function edit() {
    if (disposed || !state.binding || state.editing) return
    editRevision = state.binding.revision
    state.conflict = false
    state.form = {
      workspace: state.binding.workspace,
      alias: state.binding.alias,
      model: state.binding.model,
      label: state.binding.label ?? "",
      systemPrompt: state.binding.system_prompt ?? "",
      prelude: state.binding.prelude ?? "",
      rateLimits:
        state.binding.rate_limits === undefined
          ? ""
          : JSON.stringify(state.binding.rate_limits, null, 2),
      default: state.binding.default,
      enabled: state.binding.enabled,
    }
    state.editing = true
  }

  async function update() {
    if (disposed || !state.binding || state.saving) return
    const request = mutation.begin()
    try {
      const form = state.form
      const response = await systemAdministration(
        `workspace-agents/${encodeURIComponent(state.binding.workspace)}/${encodeURIComponent(state.binding.id)}`,
        "PATCH",
        {
          model: form.model,
          label: optional(form.label),
          system_prompt: optional(form.systemPrompt),
          prelude: optional(form.prelude),
          rate_limits: rateLimits(form.rateLimits),
          default: form.default,
          enabled: form.enabled,
          expected_revision: editRevision,
        },
        request.signal,
      )
      if (!request.current()) return
      if (response.status === 401) onAuthenticationLost()
      else if (response.status === 403) onSystemAccessChange("denied")
      else if (response.status === 409) {
        await load(state.binding.workspace, state.binding.id)
        if (!request.current()) return
        state.error =
          "This binding changed elsewhere. Your draft is preserved; review it before retrying."
        state.conflict = true
      } else if (!response.ok)
        state.error = "Workspace agent binding could not be saved."
      else {
        const binding = (await response.json()) as SystemWorkspaceAgent
        request.commit(() => {
          resource.stop()
          state.binding = binding
          state.editing = false
        })
      }
    } catch {
      if (request.current())
        state.error = "Workspace agent binding could not be saved."
    } finally {
      request.finish()
    }
  }

  function stop() {
    disposed = true
    resource.stop()
    mutation.stop()
  }
  function replaceDraft() {
    if (disposed) return
    state.editing = false
    state.error = ""
    edit()
  }
  return { state, load, edit, update, stop, replaceDraft }
}
