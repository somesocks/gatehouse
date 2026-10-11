<script lang="ts">
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    systemAdministration,
    updateSystemPrincipal,
    type SystemPrincipal,
  } from "../../../app/system"
  let {
    principalID,
    editingPage = false,
  }: { principalID: string; editingPage?: boolean } = $props()
  const runtime = useRuntime()
  let principal = $state<SystemPrincipal | null>(null)
  let enabled = $state(true),
    loading = $state(true),
    saving = $state(false),
    error = $state("")
  let generation = 0
  $effect(() => {
    const id = principalID
    const current = ++generation
    const request = new AbortController()
    principal = null
    loading = true
    error = ""
    void systemAdministration(
      `principals/${encodeURIComponent(id)}`,
      "GET",
      undefined,
      request.signal,
    )
      .then(async (response) => {
        if (current !== generation) return
        if (response.status === 401) runtime.requireLogin()
        else if (response.status === 403)
          runtime.access.setSystemAccess("denied")
        else if (!response.ok) error = "Principal could not be loaded."
        else {
          const result = (await response.json()) as SystemPrincipal
          if (current === generation) {
            principal = result
            enabled = result.enabled
          }
        }
      })
      .catch(() => {
        if (!request.signal.aborted) error = "Principal could not be loaded."
      })
      .finally(() => {
        if (current === generation) loading = false
      })
    return () => {
      generation++
      request.abort()
    }
  })
  async function save() {
    if (saving || !principal) return
    if (
      !enabled &&
      !window.confirm(
        `Disable ${principal.name ?? principal.alias ?? principal.id}?`,
      )
    )
      return
    const current = generation
    saving = true
    error = ""
    try {
      const response = await updateSystemPrincipal(principalID, enabled)
      if (current !== generation) return
      if (response.status === 401) runtime.requireLogin()
      else if (response.status === 403) runtime.access.setSystemAccess("denied")
      else if (!response.ok) error = "Principal could not be saved."
      else {
        if (
          !enabled &&
          runtime.auth.state.claims?.principal.ref.id === principalID
        )
          runtime.requireLogin()
        else
          runtime.navigate(
            `/app/system/principals/${encodeURIComponent(principalID)}`,
            true,
          )
      }
    } catch {
      if (current === generation) error = "Principal could not be saved."
    } finally {
      if (current === generation) saving = false
    }
  }
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "principals")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Principals", href: "/app/system/principals" },
    {
      label: principal?.name ?? principal?.alias ?? "Principal",
      href: editingPage
        ? `/app/system/principals/${encodeURIComponent(principalID)}`
        : undefined,
    },
    ...(editingPage ? [{ label: "Edit" }] : []),
  ]}
>
  <UI.AdminContent mode={editingPage ? "form" : "detail"}>
    {#if loading}<UI.Text tone="muted">Loading principal…</UI.Text
      >{:else if !principal}<UI.Notice variant="destructive" role="alert"
        >{error || "Principal was not found."}</UI.Notice
      >
    {:else if editingPage}<UI.AdminHeader
        title={`Edit ${principal.name ?? principal.alias ?? "principal"}`}
        id={principalID}
        description="Change account availability. Identity associations are read-only."
      />
      <UI.Form
        surface
        onsubmit={(event) => {
          event.preventDefault()
          void save()
        }}
        ><UI.FormSection title="Availability"
          ><UI.Label layout="choice"
            ><UI.Checkbox bind:checked={enabled} />Enabled</UI.Label
          ></UI.FormSection
        >{#if error}<UI.Notice variant="destructive" role="alert"
            >{error}</UI.Notice
          >{/if}<UI.FormActions
          ><UI.RouterLink
            variant="outline"
            href={`/app/system/principals/${encodeURIComponent(principalID)}`}
            >Cancel</UI.RouterLink
          ><UI.Button type="submit" disabled={saving}
            >{saving ? "Saving…" : "Save changes"}</UI.Button
          ></UI.FormActions
        ></UI.Form
      >
    {:else}<UI.AdminHeader
        kind="Principal"
        title={principal.name ?? principal.alias ?? principal.id}
        id={principal.id}
        enabled={principal.enabled}
        >{#snippet actions()}<UI.RouterLink
            variant="outline"
            href={`/app/system/principals/${encodeURIComponent(principalID)}/edit`}
            >Edit principal</UI.RouterLink
          >{/snippet}</UI.AdminHeader
      >
      <UI.DetailSection title="Account"
        ><UI.Property label="Display name"
          >{principal.name ?? "Not configured"}</UI.Property
        ><UI.Property label="Alias"
          >{principal.alias ?? "Not configured"}</UI.Property
        ><UI.Property label="Revision">{principal.revision}</UI.Property
        ></UI.DetailSection
      >
      <UI.Stack
        ><UI.Heading level={3}>Identities</UI.Heading><UI.RecordTable
          headers={["Identity", "Status", "Revision"]}
          >{#each principal.identities as identity (identity.id)}<UI.RecordRow
              ><UI.RecordCell primary
                ><UI.Text emphasis="strong">{identity.key}</UI.Text
                ><UI.Identifier value={identity.id} /></UI.RecordCell
              ><UI.RecordCell label="Status"
                ><UI.StatusBadge enabled={identity.enabled} /></UI.RecordCell
              ><UI.RecordCell label="Revision"
                >{identity.revision}</UI.RecordCell
              ></UI.RecordRow
            >{:else}<UI.TableEmpty
              columns={3}
              message="No identity associations."
            />{/each}</UI.RecordTable
        ></UI.Stack
      >
    {/if}
  </UI.AdminContent>
</UI.AppFrame>
