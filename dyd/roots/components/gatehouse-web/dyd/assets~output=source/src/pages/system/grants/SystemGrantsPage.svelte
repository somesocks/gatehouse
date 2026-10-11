<script lang="ts">
  import * as UI from "../../../ui"
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
    return controller.stop
  })
  function name(id: string) {
    const principal = subjects.find((p) => p.id === id)
    return principal?.name ?? principal?.alias ?? id
  }
  async function setEnabled(grant: SystemGrant) {
    if (updating.has(grant.ref.id)) return
    if (
      grant.enabled &&
      !window.confirm(
        `Revoke system-manager access for ${name(grant.principal.id)}?`,
      )
    )
      return
    updating = new Set(updating).add(grant.ref.id)
    mutationError = ""
    try {
      const response = await updateSystemGrant(grant.ref.id, !grant.enabled)
      if (response.status === 401) runtime.requireLogin()
      else if (response.status === 403) runtime.access.setSystemAccess("denied")
      else if (!response.ok) mutationError = "Grant could not be updated."
      else await controller.reload()
    } catch {
      mutationError = "Grant could not be updated."
    } finally {
      const next = new Set(updating)
      next.delete(grant.ref.id)
      updating = next
    }
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
      loading={controller.state.loading}
      onsearch={() => void controller.search()}
    />
    {#if controller.state.error || mutationError}<UI.Notice
        variant="destructive"
        role="alert">{mutationError || controller.state.error}</UI.Notice
      >{/if}
    <UI.RecordTable headers={["Principal", "Role", "Grant status", "Actions"]}>
      {#if controller.state.loading}<UI.TableEmpty
          columns={4}
          message="Loading grants…"
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
                onclick={() => void setEnabled(grant)}
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
      loading={controller.state.loading}
      onprevious={() => void controller.previous()}
      onnext={() => void controller.next()}
    />
  </UI.AdminContent>
</UI.AppFrame>
