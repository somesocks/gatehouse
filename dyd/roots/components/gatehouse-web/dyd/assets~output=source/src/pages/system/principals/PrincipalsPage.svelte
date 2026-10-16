<script lang="ts">
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    searchSystemPrincipals,
    type SystemPrincipal,
  } from "../../../app/system"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import { onMount } from "svelte"
  const runtime = useRuntime()
  const controller = createDirectoryController<SystemPrincipal>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await searchSystemPrincipals(
        name,
        cursor,
        status,
        signal,
      )
      const result = response.ok
        ? ((await response.json()) as {
            principals: SystemPrincipal[]
            next_cursor?: string
          })
        : undefined
      return { response, items: result?.principals, next: result?.next_cursor }
    },
  })
  onMount(() => {
    void controller.search()
    return controller.stop
  })
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "principals")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Principals" },
  ]}
>
  <UI.AdminContent
    ><UI.AdminHeader
      title="Principals"
      description="Create accounts and manage their status and login identities."
      >{#snippet actions()}<UI.RouterLink
          variant="default"
          href="/app/system/principals/new">Create principal</UI.RouterLink
        >{/snippet}</UI.AdminHeader
    >
    <UI.SearchToolbar
      bind:value={controller.state.search}
      bind:status={controller.state.status}
      statuses
      loading={controller.state.busy}
      onsearch={() => void controller.search()}
    />
    {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
        >{controller.state.error}</UI.Notice
      >{/if}
    <UI.RecordTable headers={["Principal", "Alias", "Identities", "Status"]}>
      {#if !controller.state.hasLoaded}<UI.TableEmpty
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
            >
            <UI.RecordCell label="Alias"
              >{principal.alias ?? "Not configured"}</UI.RecordCell
            >
            <UI.RecordCell label="Identities"
              >{principal.identity_count ??
                principal.identities.length}</UI.RecordCell
            >
            <UI.RecordCell label="Status"
              ><UI.StatusBadge enabled={principal.enabled} /></UI.RecordCell
            >
          </UI.RecordRow>{:else}<UI.TableEmpty
            columns={4}
            message="No principals match your search."
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
