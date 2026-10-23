<script lang="ts">
  import * as UI from "../../../ui"
  import { createConfirmation } from "../../../utils/confirmation.svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    fetchSystemWorkspacePrincipals,
    removeSystemWorkspacePrincipalGrants,
    type SystemWorkspacePrincipal,
  } from "../../../app/system"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import SystemWorkspaceFrame from "./SystemWorkspaceFrame.svelte"
  import { createWorkspaceGrantRoleController } from "./workspace-grant-role-controller.svelte"
  import { onMount } from "svelte"
  import { reducesRoleAccess } from "../../../app/role-access"
  let { workspaceID }: { workspaceID: string } = $props()
  const runtime = useRuntime()
  const confirmation = createConfirmation()
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
  const roleEditor = createWorkspaceGrantRoleController({
    workspaceID: () => workspaceID,
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    onReload: () => controller.reload(),
  })
  onMount(() => {
    void controller.search()
    return () => {
      lifetime.abort()
      controller.stop()
      roleEditor.stop()
    }
  })
  async function remove(principal: SystemWorkspacePrincipal) {
    if (removing.has(principal.id) || roleEditor.state.saving) return
    if (roleEditor.state.grant?.principal === principal.id) roleEditor.cancel()
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
  function requestRemove(principal: SystemWorkspacePrincipal) {
    const snapshot = { ...principal }
    confirmation.request({
      title: "Remove workspace access?",
      description: `Remove all direct workspace grants for ${principal.name ?? principal.alias ?? principal.id}? Workspace access will be suspended until admission is restored. Retained project and chat grants are not deleted.`,
      confirmLabel: "Remove direct access",
      onConfirm: async () => { await remove(snapshot); return error },
    })
  }
  function saveRole() {
    const grant = roleEditor.state.grant
    if (!grant || !reducesRoleAccess(grant.role, roleEditor.state.role)) {
      void roleEditor.save()
      return
    }
    confirmation.request({
      title: "Reduce workspace access?",
      description: `Change ${roleEditor.state.recipient} from ${grant.role} to ${roleEditor.state.role}? Permissions provided by this grant will be reduced. Other grants still apply.`,
      confirmLabel: "Change role",
      onConfirm: async () => {
        await roleEditor.save()
        if (roleEditor.state.grant) return roleEditor.state.error || "The role could not be changed."
      },
    })
  }
</script>

<SystemWorkspaceFrame {workspaceID} breadcrumbs={[{ label: "Principals" }]}>
  <UI.AdminContent
    ><UI.AdminHeader
      title="Direct principals"
      description="Workspace membership and roles are assigned directly to each person."
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
    {#if roleEditor.state.notice}<UI.Text role="status"
        >{roleEditor.state.notice}</UI.Text
      >{/if}
    {#if roleEditor.state.grant}
      <UI.Form
        surface
        onsubmit={(event) => {
          event.preventDefault()
          saveRole()
        }}
      >
        <UI.FormSection
          title={`Edit grant role for ${roleEditor.state.recipient}`}
        >
          <UI.Identifier value={roleEditor.state.grant.id} />
          <UI.Field>
            <UI.Label for="workspace-grant-role">Role</UI.Label>
            <UI.Select
              id="workspace-grant-role"
              bind:value={roleEditor.state.role}
              disabled={roleEditor.state.saving}
            >
              <UI.Option value="member">Member</UI.Option>
              <UI.Option value="contributor">Contributor</UI.Option>
              <UI.Option value="manager">Manager</UI.Option>
            </UI.Select>
          </UI.Field>
          <UI.Text size="small" tone="muted"
            >Changes this grant only. Other direct
            grants still apply.</UI.Text
          >
        </UI.FormSection>
        {#if roleEditor.state.error}<UI.Notice
            variant="destructive"
            role="alert">{roleEditor.state.error}</UI.Notice
          >{/if}
        <UI.FormActions>
          <UI.Button
            type="button"
            variant="outline"
            disabled={roleEditor.state.saving}
            onclick={roleEditor.cancel}>Cancel</UI.Button
          >
          <UI.Button
            type="submit"
            disabled={roleEditor.state.saving || roleEditor.state.conflict}
            >{roleEditor.state.saving ? "Saving…" : "Save role"}</UI.Button
          >
        </UI.FormActions>
      </UI.Form>
    {/if}
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
                >{#each principal.grants as grant (grant.id)}<UI.Row>
                    <UI.Badge>{grant.role}</UI.Badge>
                    <UI.Button
                      size="sm"
                      variant="outline"
                      aria-label={`Edit ${grant.role} grant for ${principal.name ?? principal.alias ?? principal.id}`}
                      disabled={roleEditor.state.saving ||
                        removing.has(principal.id)}
                      onclick={() =>
                        roleEditor.begin(
                          grant,
                          principal.name ?? principal.alias ?? principal.id,
                        )}>Edit role</UI.Button
                    >
                  </UI.Row>{/each}</UI.Row
              ></UI.RecordCell
            ><UI.RecordCell label="Account status"
              ><UI.StatusBadge enabled={principal.enabled} /></UI.RecordCell
            ><UI.RecordCell actions
              ><UI.Button
                size="sm"
                variant="outline"
                disabled={removing.has(principal.id) || roleEditor.state.saving}
                onclick={() => requestRemove(principal)}
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

<UI.ConfirmationDialog {confirmation} contextKey={runtime.state.route} />
