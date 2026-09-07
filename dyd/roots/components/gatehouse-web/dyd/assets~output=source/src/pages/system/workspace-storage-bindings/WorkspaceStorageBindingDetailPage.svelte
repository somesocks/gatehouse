<script lang="ts">
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { systemAdministration, type SystemWorkspaceStorageProvider } from "../../../app/system"

  let { workspaceID, providerID }: { workspaceID: string; providerID: string } = $props()
  const runtime = useRuntime()
  let binding = $state<SystemWorkspaceStorageProvider | null>(null)
  let priority = $state(0)
  let enabled = $state(true)
  let editing = $state(false)
  let error = $state("")
  let saving = $state(false)
  let loading = $state(true)

  async function load(): Promise<void> {
    loading = true
    error = ""
    try {
      const response = await systemAdministration(`workspace-storage-providers/${encodeURIComponent(workspaceID)}/${encodeURIComponent(providerID)}`)
      if (response.status === 401) runtime.requireLogin()
      else if (response.status === 403) runtime.access.setSystemAccess("denied")
      else if (response.status === 404) error = "Workspace storage binding was not found."
      else if (!response.ok) error = "Workspace storage binding could not be loaded."
      else binding = await response.json() as SystemWorkspaceStorageProvider
    } catch { error = "Workspace storage binding could not be loaded." } finally { loading = false }
  }

  async function save(): Promise<void> {
    if (binding === null) return
    saving = true
    error = ""
    try {
      const response = await systemAdministration(`workspace-storage-providers/${encodeURIComponent(binding.workspace)}/${encodeURIComponent(binding.provider)}`, "PATCH", { priority, enabled, expected_revision: binding.revision })
      if (response.status === 409) {
        await load()
        error = "This binding changed elsewhere. The latest settings have been reloaded."
      } else if (!response.ok) error = "Workspace storage binding could not be saved."
      else { binding = await response.json() as SystemWorkspaceStorageProvider; editing = false }
    } catch { error = "Workspace storage binding could not be saved." } finally { saving = false }
  }
  $effect(() => { void load() })
</script>

<SystemFrame active="workspace-storage-bindings" title="Workspace storage binding">
  <section class="system-page">
    {#if loading}
      <p class="dashboard-empty">Loading binding...</p>
    {:else if binding === null}
      <p class="help is-danger">{error || "Workspace storage binding was not found."}</p>
      <RouterLink class="button" href="/app/system/workspace-storage-bindings">Back to bindings</RouterLink>
    {:else if !editing}
      <div class="system-page-heading mb-5"><div><p class="eyebrow">Workspace storage binding</p><h2 class="title is-3">{binding.workspace} / {binding.provider}</h2></div><button class="button is-primary" type="button" onclick={() => { priority = binding!.priority; enabled = binding!.enabled; editing = true }}>Edit</button></div>
      <dl><div class="field"><dt class="label">Priority</dt><dd>{binding.priority}</dd></div><div class="field"><dt class="label">Status</dt><dd>{binding.enabled ? "Enabled" : "Disabled"}</dd></div><div class="field"><dt class="label">Revision</dt><dd>{binding.revision}</dd></div></dl>
    {:else}
      <div class="system-page-heading mb-5"><div><p class="eyebrow">Workspace storage binding</p><h2 class="title is-3">Edit binding</h2></div></div>
      <form onsubmit={(event) => { event.preventDefault(); void save() }}><div class="field"><label class="checkbox"><input type="checkbox" bind:checked={enabled} /> Enabled</label></div><div class="field"><label class="label" for="storage-binding-priority">Priority</label><div class="control"><input class="input" id="storage-binding-priority" type="number" bind:value={priority} /></div></div><div class="field is-grouped"><p class="control"><button class="button is-primary" disabled={saving}>Save changes</button></p><p class="control"><button class="button" type="button" onclick={() => editing = false}>Cancel</button></p></div></form>
      {#if error}<p class="help is-danger">{error}</p>{/if}
    {/if}
  </section>
</SystemFrame>
