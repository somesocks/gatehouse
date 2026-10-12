<script lang="ts">
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    fetchSystemWorkspacePrincipals,
    removeSystemWorkspacePrincipalGrants,
    type SystemWorkspacePrincipal,
  } from "../../../app/system"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import SystemWorkspaceFrame from "./SystemWorkspaceFrame.svelte"
  import { onMount } from "svelte"
  let { workspaceID }: { workspaceID: string } = $props()
  const runtime = useRuntime()
  const lifetime = new AbortController()
  let removing = $state(new Set<string>()),
    error = $state("")
  const controller = createDirectoryController<SystemWorkspacePrincipal>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await fetchSystemWorkspacePrincipals(
        workspaceID,
        name,
        cursor,
        signal,
      )
      const result = response.ok
        ? ((await response.json()) as {
            principals: SystemWorkspacePrincipal[]
            next_cursor?: string
          })
        : undefined
      return { response, items: result?.principals, next: result?.next_cursor }
    },
  })
  onMount(() => {
    void controller.search()
    return () => {
      lifetime.abort()
      controller.stop()
    }
  })
  async function remove(principal: SystemWorkspacePrincipal) {
    if (
      removing.has(principal.id) ||
      !window.confirm(
        `Remove direct workspace access for ${principal.name ?? principal.alias ?? principal.id}? Group-derived access may remain.`,
      )
    )
      return
    removing = new Set(removing).add(principal.id)
    error = ""
    try {
      const response = await removeSystemWorkspacePrincipalGrants(
        workspaceID,
        principal.id,
        lifetime.signal,
      )
      if (lifetime.signal.aborted) return
      if (response.status === 401) runtime.requireLogin()
      else if (response.status === 403) runtime.access.setSystemAccess("denied")
      else if (!response.ok) error = "Direct access could not be removed."
      else await controller.reload()
    } catch {
      if (!lifetime.signal.aborted)
        error = "Direct access could not be removed."
    } finally {
      if (!lifetime.signal.aborted) {
        const next = new Set(removing)
        next.delete(principal.id)
        removing = next
      }
    }
  }
</script>

<SystemWorkspaceFrame {workspaceID} breadcrumbs={[{ label: "Principals" }]}>
  <UI.AdminContent
    ><UI.AdminHeader
      title="Direct principals"
      description="Direct workspace grants. Access inherited through groups is managed separately."
      >{#snippet actions()}<UI.RouterLink
          variant="default"
          href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/principals/new`}
          >Add principal</UI.RouterLink
        >{/snippet}</UI.AdminHeader
    >
    <UI.SearchToolbar
      bind:value={controller.state.search}
      loading={controller.state.busy}
      onsearch={() => void controller.search()}
    />
    {#if controller.state.error || error}<UI.Notice
        variant="destructive"
        role="alert">{error || controller.state.error}</UI.Notice
      >{/if}
    <UI.RecordTable
      headers={["Principal", "Direct roles", "Account status", "Actions"]}
      >{#if !controller.state.hasLoaded}<UI.TableEmpty
          columns={4}
          message={controller.state.error
            ? "Principals could not be loaded."
            : "Loading principals…"}
        />{:else}{#each controller.state.items as principal (principal.id)}<UI.RecordRow
          >
            <UI.RecordCell primary
              ><UI.RouterLink
                href={`/app/system/principals/${encodeURIComponent(principal.id)}`}
                >{principal.name ??
                  principal.alias ??
                  principal.id}</UI.RouterLink
              ><UI.Identifier
                value={principal.id}
                copyable={false}
              /></UI.RecordCell
            ><UI.RecordCell label="Direct roles"
              ><UI.Row
                >{#each principal.roles as role}<UI.Badge>{role}</UI.Badge
                  >{/each}</UI.Row
              ></UI.RecordCell
            ><UI.RecordCell label="Account status"
              ><UI.StatusBadge enabled={principal.enabled} /></UI.RecordCell
            ><UI.RecordCell actions
              ><UI.Button
                size="sm"
                variant="outline"
                disabled={removing.has(principal.id)}
                onclick={() => void remove(principal)}
                >{removing.has(principal.id)
                  ? "Removing…"
                  : "Remove direct access"}</UI.Button
              ></UI.RecordCell
            >
          </UI.RecordRow>{:else}<UI.TableEmpty
            columns={4}
            message="No direct grants match your search."
          />{/each}{/if}</UI.RecordTable
    >
    <UI.PaginationFooter
      count={controller.state.items.length}
      previous={!!controller.state.history.length}
      next={!!controller.state.next}
      loading={controller.state.busy}
      onprevious={() => void controller.previous()}
      onnext={() => void controller.next()}
    />
  </UI.AdminContent>
</SystemWorkspaceFrame>
