<script lang="ts">
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { onMount, untrack } from "svelte"
  import { createStorageProviderListController } from "./storage-provider-list-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createStorageProviderListController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  onMount(() => controller.start())
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "storage-providers")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Storage providers" },
  ]}
>
  <UI.AdminContent
    ><UI.AdminHeader
      title="Storage providers"
      description="Embedded and S3 storage available for workspace bindings."
      >{#snippet actions()}<UI.RouterLink
          variant="default"
          href="/app/system/storage-providers/new"
          >Create provider</UI.RouterLink
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
    <UI.RecordTable headers={["Provider", "Protocol", "Bucket", "Status"]}>
      {#if !controller.state.hasLoaded}<UI.TableEmpty
          columns={4}
          message={controller.state.error
            ? "Providers could not be loaded."
            : "Loading providers…"}
        />{:else}{#each controller.state.items as provider (provider.id)}<UI.RecordRow
          >
            <UI.RecordCell primary
              ><UI.RouterLink
                href={`/app/system/storage-providers/${encodeURIComponent(provider.id)}`}
                >{provider.alias}</UI.RouterLink
              ><UI.Identifier
                value={provider.id}
                copyable={false}
              /></UI.RecordCell
            >
            <UI.RecordCell label="Protocol">{provider.protocol}</UI.RecordCell>
            <UI.RecordCell label="Bucket"
              >{provider.bucket ?? "Not applicable"}</UI.RecordCell
            >
            <UI.RecordCell label="Status"
              ><UI.StatusBadge enabled={provider.enabled} /></UI.RecordCell
            >
          </UI.RecordRow>{:else}<UI.TableEmpty
            columns={4}
            message="No providers match your search."
          />{/each}{/if}
    </UI.RecordTable>
    <UI.PaginationFooter
      count={controller.state.items.length}
      previous={controller.state.history.length > 0}
      next={!!controller.state.next}
      loading={controller.state.busy}
      onprevious={() => void controller.previous()}
      onnext={() => void controller.next()}
    />
  </UI.AdminContent>
</UI.AppFrame>
