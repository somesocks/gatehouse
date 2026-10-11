<script lang="ts">
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    systemAdministration,
    type SystemWorkspaceStorageProvider,
    type SystemStorageProvider,
  } from "../../../app/system"
  import SystemWorkspaceFrame from "../workspaces/SystemWorkspaceFrame.svelte"
  let {
    workspaceID,
    providerID,
    editingPage = false,
  }: {
    workspaceID: string
    providerID: string
    editingPage?: boolean
  } = $props()
  const runtime = useRuntime()
  let binding = $state<SystemWorkspaceStorageProvider | null>(null),
    provider = $state<SystemStorageProvider | null>(null)
  let priority = $state(1),
    enabled = $state(true),
    loading = $state(true),
    saving = $state(false),
    error = $state("")
  let generation = 0
  async function load(signal?: AbortSignal) {
    loading = true
    error = ""
    const current = generation
    try {
      const response = await systemAdministration(
        `workspace-storage-providers/${encodeURIComponent(workspaceID)}/${encodeURIComponent(providerID)}`,
        "GET",
        undefined,
        signal,
      )
      if (current !== generation || signal?.aborted) return
      if (response.status === 401) runtime.requireLogin()
      else if (response.status === 403) runtime.access.setSystemAccess("denied")
      else if (!response.ok) error = "Binding could not be loaded."
      else {
        const result = (await response.json()) as SystemWorkspaceStorageProvider
        if (current === generation) {
          binding = result
          priority = result.priority
          enabled = result.enabled
        }
      }
    } catch {
      if (!signal?.aborted && current === generation)
        error = "Binding could not be loaded."
    } finally {
      if (current === generation) loading = false
    }
  }
  $effect(() => {
    const workspace = workspaceID
    const id = providerID
    generation++
    binding = null
    provider = null
    const request = new AbortController()
    void load(request.signal)
    void systemAdministration(
      `storage-providers/${encodeURIComponent(id)}`,
      "GET",
      undefined,
      request.signal,
    )
      .then(async (response) => {
        if (response.ok) {
          const result = (await response.json()) as SystemStorageProvider
          if (!request.signal.aborted) provider = result
        }
      })
      .catch(() => {})
    return () => {
      generation++
      request.abort()
    }
  })
  async function save() {
    if (!binding || saving) return
    const current = generation
    saving = true
    error = ""
    try {
      const response = await systemAdministration(
        `workspace-storage-providers/${encodeURIComponent(workspaceID)}/${encodeURIComponent(providerID)}`,
        "PATCH",
        { priority, enabled, expected_revision: binding.revision },
      )
      if (current !== generation) return
      if (response.status === 401) runtime.requireLogin()
      else if (response.status === 403) runtime.access.setSystemAccess("denied")
      else if (response.status === 409) {
        await load()
        error =
          "This binding changed elsewhere. The latest settings have been reloaded."
      } else if (!response.ok) error = "Binding could not be saved."
      else
        runtime.navigate(
          `/app/system/workspaces/${encodeURIComponent(workspaceID)}/storage-bindings/${encodeURIComponent(providerID)}`,
          true,
        )
    } catch {
      if (current === generation) error = "Binding could not be saved."
    } finally {
      if (current === generation) saving = false
    }
  }
</script>

<SystemWorkspaceFrame
  {workspaceID}
  breadcrumbs={[
    {
      label: "Storage bindings",
      href: `/app/system/workspaces/${encodeURIComponent(workspaceID)}/storage-bindings`,
    },
    {
      label: provider?.alias ?? "Binding",
      href: editingPage
        ? `/app/system/workspaces/${encodeURIComponent(workspaceID)}/storage-bindings/${encodeURIComponent(providerID)}`
        : undefined,
    },
    ...(editingPage ? [{ label: "Edit" }] : []),
  ]}
>
  <UI.AdminContent mode={editingPage ? "form" : "detail"}>
    {#if loading}<UI.Text tone="muted">Loading binding…</UI.Text
      >{:else if !binding}<UI.Notice variant="destructive" role="alert"
        >{error || "Binding was not found."}</UI.Notice
      >
    {:else if editingPage}<UI.AdminHeader
        title={`Edit ${provider?.alias ?? "storage binding"}`}
        description="Update storage priority and availability."
      />
      <UI.Form
        surface
        onsubmit={(event) => {
          event.preventDefault()
          void save()
        }}
        ><UI.FormSection title="Provider assignment"
          ><UI.Text emphasis="strong">{provider?.alias ?? providerID}</UI.Text
          ><UI.Identifier value={providerID} /></UI.FormSection
        ><UI.FormSection title="Priority"
          ><UI.Field
            ><UI.Label for="storage-priority">Priority</UI.Label><UI.Input
              id="storage-priority"
              type="number"
              min="1"
              required
              bind:value={priority}
            /></UI.Field
          ></UI.FormSection
        ><UI.FormSection title="Availability"
          ><UI.Label layout="choice"
            ><UI.Checkbox bind:checked={enabled} />Enabled</UI.Label
          ></UI.FormSection
        >{#if error}<UI.Notice variant="destructive" role="alert"
            >{error}</UI.Notice
          >{/if}<UI.FormActions
          ><UI.RouterLink
            variant="outline"
            href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/storage-bindings/${encodeURIComponent(providerID)}`}
            >Cancel</UI.RouterLink
          ><UI.Button type="submit" disabled={saving}
            >{saving ? "Saving…" : "Save changes"}</UI.Button
          ></UI.FormActions
        ></UI.Form
      >
    {:else}<UI.AdminHeader
        kind="Storage binding"
        title={provider?.alias ?? "Storage binding"}
        enabled={binding.enabled}
        >{#snippet actions()}<UI.RouterLink
            variant="outline"
            href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/storage-bindings/${encodeURIComponent(providerID)}/edit`}
            >Edit binding</UI.RouterLink
          >{/snippet}</UI.AdminHeader
      >
      <UI.DetailSection title="Provider assignment"
        ><UI.Property label="Provider"
          ><UI.RouterLink
            href={`/app/system/storage-providers/${encodeURIComponent(providerID)}`}
            >{provider?.alias ?? providerID}</UI.RouterLink
          ></UI.Property
        ><UI.Property label="Priority">{binding.priority}</UI.Property
        ></UI.DetailSection
      ><UI.DetailSection title="Record information"
        ><UI.Property label="Provider ID"
          ><UI.Identifier value={providerID} /></UI.Property
        ><UI.Property label="Revision">{binding.revision}</UI.Property
        ></UI.DetailSection
      >
    {/if}
  </UI.AdminContent>
</SystemWorkspaceFrame>
