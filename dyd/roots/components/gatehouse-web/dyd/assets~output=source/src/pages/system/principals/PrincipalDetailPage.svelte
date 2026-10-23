<script lang="ts">
  import { untrack } from "svelte"
  import { createLoadState } from "../../../utils/load-state.svelte"
  import * as UI from "../../../ui"
  import { createConfirmation } from "../../../utils/confirmation.svelte"
  import PrincipalLogins from "./PrincipalLogins.svelte"
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
  const confirmation = createConfirmation()
  const resource = createLoadState()
  const mutation = createLoadState()
  let principal = $state<SystemPrincipal | null>(null)
  let enabled = $state(true),
    error = $state("")
  const disabling = $derived(principal?.enabled && !enabled)
  const loading = $derived(resource.loading)
  const saving = $derived(mutation.state.pending)
  let generation = 0
  async function load() {
    const current = generation
    const request = resource.begin()
    error = ""
    try {
      const response = await systemAdministration(
        `principals/${encodeURIComponent(principalID)}`,
        "GET",
        undefined,
        request.signal,
      )
      if (current !== generation || !request.current()) return
      if (response.status === 401) runtime.requireLogin()
      else if (response.status === 403) runtime.access.setSystemAccess("denied")
      else if (!response.ok) throw new Error("Principal could not be loaded.")
      else {
        const result = (await response.json()) as SystemPrincipal
        const first = !resource.state.hasLoaded
        request.commit(() => {
          principal = result
          if (first) enabled = result.enabled
        })
      }
    } catch (cause) {
      request.fail(cause)
    } finally {
      request.finish()
    }
  }
  $effect(() => {
    principalID
    editingPage
    untrack(() => {
      generation++
      resource.reset()
      mutation.stop()
      principal = null
      enabled = true
      void load()
    })
    return () => {
      generation++
      resource.stop()
      mutation.stop()
    }
  })
  async function save() {
    if (saving || !principal) return
    const current = generation
    const request = mutation.begin()
    error = ""
    try {
      const response = await updateSystemPrincipal(
        principalID,
        enabled,
        request.signal,
      )
      if (current !== generation || !request.current()) return
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
      if (current === generation && request.current())
        error = "Principal could not be saved."
    } finally {
      request.finish()
    }
  }
  function requestSave() {
    if (!disabling) { void save(); return }
    confirmation.request({
      title: "Disable principal?",
      description: `Disable ${principal?.name ?? principal?.alias ?? "this principal"}? The account will lose access until enabled again. Disabling your own account signs you out.`,
      confirmLabel: "Disable principal",
      onConfirm: async () => { await save(); return error },
    })
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
    {#if resource.reloading}<UI.Text role="status"
        >Refreshing principal…</UI.Text
      >{/if}
    {#if resource.state.error}<UI.Notice variant="destructive" role="alert"
        >{resource.state.error}<UI.Button
          variant="outline"
          onclick={() => void load()}>Retry</UI.Button
        ></UI.Notice
      >{/if}
    {#if loading}<UI.Text tone="muted">Loading principal…</UI.Text
      >{:else if !principal}<UI.Notice variant="destructive" role="alert"
        >{error || "Principal was not found."}</UI.Notice
      >
    {:else if editingPage}<UI.AdminHeader
        title={`Edit ${principal.name ?? principal.alias ?? "principal"}`}
        id={principalID}
        description="Change account availability. Manage logins from the principal detail page."
      />
      <UI.Form
        surface
        onsubmit={(event) => {
          event.preventDefault()
          requestSave()
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
      <PrincipalLogins
        {principal}
        onChanged={(next) => {
          resource.stop()
          principal = next
        }}
        onReload={load}
      />
    {/if}
  </UI.AdminContent>
</UI.AppFrame>

<UI.ConfirmationDialog {confirmation} contextKey={runtime.state.route} />
