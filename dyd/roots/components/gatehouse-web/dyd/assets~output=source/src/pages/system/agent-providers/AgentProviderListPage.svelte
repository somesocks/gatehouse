<script lang="ts">
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { onMount, untrack } from "svelte"
  import { createAgentProviderListController } from "./agent-provider-list-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createAgentProviderListController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  onMount(() => controller.start())
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "agent-providers")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Agent providers" },
  ]}
>
  <UI.AdminContent
    ><UI.AdminHeader
      title="Agent providers"
      description="Services that supply agent models."
      >{#snippet actions()}<UI.Link
          variant="outline"
          href="/app/login/openai-codex"
          target="_blank"
          rel="noopener noreferrer">Codex login</UI.Link
        ><UI.RouterLink variant="default" href="/app/system/agent-providers/new"
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
    <UI.RecordTable headers={["Provider", "Protocol", "Credentials", "Status"]}>
      {#if !controller.state.hasLoaded}<UI.TableEmpty
          columns={4}
          message={controller.state.error
            ? "Providers could not be loaded."
            : "Loading providers…"}
        />{:else}{#each controller.state.items as provider (provider.id)}<UI.RecordRow
          >
            <UI.RecordCell primary
              ><UI.RouterLink
                href={`/app/system/agent-providers/${encodeURIComponent(provider.id)}`}
                >{provider.alias}</UI.RouterLink
              ><UI.Identifier
                value={provider.id}
                copyable={false}
              /></UI.RecordCell
            >
            <UI.RecordCell label="Protocol">{provider.protocol}</UI.RecordCell>
            <UI.RecordCell label="Credentials"
              >{provider.credential_configured
                ? "Configured"
                : provider.protocol === "builtin"
                  ? "Not required"
                  : "Not configured"}</UI.RecordCell
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
