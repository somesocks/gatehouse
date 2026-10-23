<script lang="ts">
  import * as UI from "../../../ui"
  import { createConfirmation } from "../../../utils/confirmation.svelte"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    searchSystemGrants,
    updateSystemGrant,
    type SystemGrant,
    type SystemPrincipal,
  } from "../../../app/system"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import { onMount } from "svelte"
  const runtime = useRuntime()
  const confirmation = createConfirmation()
  const lifetime = new AbortController()
  let subjects = $state<SystemPrincipal[]>([])
  let updating = $state(new Set<string>())
  let mutationError = $state("")
  const controller = createDirectoryController<SystemGrant>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await searchSystemGrants(name, cursor, status, signal)
      const result = response.ok
        ? ((await response.json()) as {
            grants: SystemGrant[]
            principals: SystemPrincipal[]
            next_cursor?: string
          })
        : undefined
      if (!signal.aborted && result) subjects = result.principals
      return { response, items: result?.grants, next: result?.next_cursor }
    },
  })
  onMount(() => {
    void controller.search()
    return () => {
      lifetime.abort()
      controller.stop()
    }
  })
  function name(id: string) {
    const principal = subjects.find((p) => p.id === id)
    return principal?.name ?? principal?.alias ?? id
  }
  async function setEnabled(grant: SystemGrant) {
    if (updating.has(grant.ref.id)) return
    updating = new Set(updating).add(grant.ref.id)
    mutationError = ""
    try {
      const response = await updateSystemGrant(
        grant.ref.id,
        !grant.enabled,
        lifetime.signal,
      )
      if (lifetime.signal.aborted) return
      if (response.status === 401) runtime.requireLogin()
      else if (response.status === 403) runtime.access.setSystemAccess("denied")
      else if (!response.ok) mutationError = "Grant could not be updated."
      else await controller.reload()
    } catch {
      if (!lifetime.signal.aborted)
        mutationError = "Grant could not be updated."
    } finally {
      if (!lifetime.signal.aborted) {
        const next = new Set(updating)
        next.delete(grant.ref.id)
        updating = next
      }
    }
  }
  function requestSetEnabled(grant: SystemGrant) {
    if (!grant.enabled) { void setEnabled(grant); return }
    const snapshot = { ...grant }
    confirmation.request({
      title: "Revoke system access?",
      description: `${name(grant.principal.id)} will lose the system-manager access provided by this grant.`,
      confirmLabel: "Revoke access",
      onConfirm: async () => { await setEnabled(snapshot); return mutationError },
    })
  }
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "grants")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "System grants" },
  ]}
>
  <UI.AdminContent
    ><UI.AdminHeader
      title="System grants"
      description="Principals permitted to manage global Gatehouse state."
      >{#snippet actions()}<UI.RouterLink
          variant="default"
          href="/app/system/grants/new">Add manager</UI.RouterLink
        >{/snippet}</UI.AdminHeader
    >
    <UI.SearchToolbar
      bind:value={controller.state.search}
      bind:status={controller.state.status}
      statuses
      loading={controller.state.busy}
      onsearch={() => void controller.search()}
    />
    {#if controller.state.error || mutationError}<UI.Notice
        variant="destructive"
        role="alert">{mutationError || controller.state.error}</UI.Notice
      >{/if}
    <UI.RecordTable headers={["Principal", "Role", "Grant status", "Actions"]}>
      {#if !controller.state.hasLoaded}<UI.TableEmpty
          columns={4}
          message={controller.state.error
            ? "Grants could not be loaded."
            : "Loading grants…"}
        />{:else}{#each controller.state.items as grant (grant.ref.id)}<UI.RecordRow
          >
            <UI.RecordCell primary
              ><UI.RouterLink
                href={`/app/system/principals/${encodeURIComponent(grant.principal.id)}`}
                >{name(grant.principal.id)}</UI.RouterLink
              ><UI.Identifier
                value={grant.principal.id}
                copyable={false}
              /></UI.RecordCell
            >
            <UI.RecordCell label="Role"
              ><UI.Badge>System manager</UI.Badge></UI.RecordCell
            ><UI.RecordCell label="Grant status"
              ><UI.StatusBadge
                enabled={grant.enabled}
                active="Active"
                inactive="Revoked"
              /></UI.RecordCell
            >
            <UI.RecordCell actions
              ><UI.Button
                size="sm"
                variant="outline"
                disabled={updating.has(grant.ref.id)}
                onclick={() => requestSetEnabled(grant)}
                >{updating.has(grant.ref.id)
                  ? "Saving…"
                  : grant.enabled
                    ? "Revoke access"
                    : "Restore access"}</UI.Button
              ></UI.RecordCell
            >
          </UI.RecordRow>{:else}<UI.TableEmpty
            columns={4}
            message="No grants match your search."
          />{/each}{/if}
    </UI.RecordTable>
    <UI.PaginationFooter
      count={controller.state.items.length}
      previous={!!controller.state.history.length}
      next={!!controller.state.next}
      loading={controller.state.busy}
      onprevious={() => void controller.previous()}
      onnext={() => void controller.next()}
    />
  </UI.AdminContent>
</UI.AppFrame>

<UI.ConfirmationDialog {confirmation} contextKey={runtime.state.route} />
