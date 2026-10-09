<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"

  import {
    systemAdministration,
    type SystemWorkspaceStorageProvider,
  } from "../../../app/system"

  let { workspaceID, providerID }: { workspaceID: string; providerID: string } =
    $props()
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
      const response = await systemAdministration(
        `workspace-storage-providers/${encodeURIComponent(workspaceID)}/${encodeURIComponent(providerID)}`,
      )
      if (response.status === 401) runtime.requireLogin()
      else if (response.status === 403) runtime.access.setSystemAccess("denied")
      else if (response.status === 404)
        error = "Workspace storage binding was not found."
      else if (!response.ok)
        error = "Workspace storage binding could not be loaded."
      else binding = (await response.json()) as SystemWorkspaceStorageProvider
    } catch {
      error = "Workspace storage binding could not be loaded."
    } finally {
      loading = false
    }
  }

  async function save(): Promise<void> {
    if (binding === null) return
    saving = true
    error = ""
    try {
      const response = await systemAdministration(
        `workspace-storage-providers/${encodeURIComponent(binding.workspace)}/${encodeURIComponent(binding.provider)}`,
        "PATCH",
        { priority, enabled, expected_revision: binding.revision },
      )
      if (response.status === 409) {
        await load()
        error =
          "This binding changed elsewhere. The latest settings have been reloaded."
      } else if (!response.ok)
        error = "Workspace storage binding could not be saved."
      else {
        binding = (await response.json()) as SystemWorkspaceStorageProvider
        editing = false
      }
    } catch {
      error = "Workspace storage binding could not be saved."
    } finally {
      saving = false
    }
  }
  $effect(() => {
    void load()
  })
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "workspace-storage-bindings")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Workspace storage binding" },
  ]}
>
  <UI.Stack as="section">
    {#if loading}
      <UI.Text as="p" tone="muted">Loading binding...</UI.Text>
    {:else if binding === null}
      <UI.Text as="p" size="small" role="alert">
        {error || "Workspace storage binding was not found."}
      </UI.Text>
      <RouterLink
        variant="outline"
        href="/app/system/workspace-storage-bindings"
        >Back to bindings</RouterLink
      >
    {:else if !editing}
      <UI.Row justify="between">
        <UI.Group>
          <UI.Text as="p" size="eyebrow">Workspace storage binding</UI.Text>
          <UI.Heading level={2}
            >{binding.workspace} / {binding.provider}</UI.Heading
          >
        </UI.Group>
        <UI.Button
          variant="outline"
          type="button"
          onclick={() => {
            priority = binding!.priority
            enabled = binding!.enabled
            editing = true
          }}>Edit</UI.Button
        >
      </UI.Row>
      <UI.Stack as="dl">
        <UI.Field>
          <UI.Text as="dt" size="small">Priority</UI.Text>
          <UI.Text as="dd">{binding.priority}</UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Status</UI.Text>
          <UI.Text as="dd">{binding.enabled ? "Enabled" : "Disabled"}</UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Revision</UI.Text>
          <UI.Text as="dd">{binding.revision}</UI.Text>
        </UI.Field>
      </UI.Stack>
    {:else}
      <UI.Stack>
        <UI.Group>
          <UI.Text as="p" size="eyebrow">Workspace storage binding</UI.Text>
          <UI.Heading level={2}>Edit binding</UI.Heading>
        </UI.Group>
      </UI.Stack>
      <UI.Form
        onsubmit={(event) => {
          event.preventDefault()
          void save()
        }}
      >
        <UI.Field>
          <UI.Label layout="choice"
            ><UI.Checkbox bind:checked={enabled} /> Enabled</UI.Label
          >
        </UI.Field>
        <UI.Field>
          <UI.Label for="storage-binding-priority">Priority</UI.Label>
          <UI.Group>
            <UI.Input
              id="storage-binding-priority"
              type="number"
              bind:value={priority}
            />
          </UI.Group>
        </UI.Field>
        <UI.Row>
          <UI.Group>
            <UI.Button variant="default" type="submit" disabled={saving}
              >Save changes</UI.Button
            >
          </UI.Group>
          <UI.Group>
            <UI.Button
              variant="outline"
              type="button"
              onclick={() => (editing = false)}>Cancel</UI.Button
            >
          </UI.Group>
        </UI.Row>
      </UI.Form>
      {#if error}<UI.Text as="p" size="small" role="alert">{error}</UI.Text
        >{/if}
    {/if}
  </UI.Stack>
</UI.AppFrame>
