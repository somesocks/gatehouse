<script lang="ts">
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { onMount, untrack } from "svelte"
  import { createAgentModelListController } from "./agent-model-list-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createAgentModelListController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  onMount(() => controller.start())
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "agent-models")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Agent models" },
  ]}
>
  <UI.AdminContent
    ><UI.AdminHeader
      title="Agent models"
      description="Models available for workspace bindings."
      >{#snippet actions()}<UI.RouterLink
          variant="default"
          href="/app/system/agent-models/new">Create model</UI.RouterLink
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
    <UI.RecordTable headers={["Alias", "Provider", "Model", "Status"]}>
      {#if !controller.state.hasLoaded}<UI.TableEmpty
          columns={4}
          message={controller.state.error
            ? "Models could not be loaded."
            : "Loading models…"}
        />{:else}{#each controller.state.items as model (model.id)}<UI.RecordRow
          >
            <UI.RecordCell primary
              ><UI.RouterLink
                href={`/app/system/agent-models/${encodeURIComponent(model.id)}`}
                >{model.alias}</UI.RouterLink
              ><UI.Identifier
                value={model.id}
                copyable={false}
              /></UI.RecordCell
            >
            <UI.RecordCell label="Provider"
              ><UI.RouterLink
                href={`/app/system/agent-providers/${encodeURIComponent(model.provider)}`}
                >{model.provider_alias ?? model.provider}</UI.RouterLink
              ></UI.RecordCell
            >
            <UI.RecordCell label="Model">{model.model}</UI.RecordCell>
            <UI.RecordCell label="Status"
              ><UI.StatusBadge enabled={model.enabled} /></UI.RecordCell
            >
          </UI.RecordRow>{:else}<UI.TableEmpty
            columns={4}
            message="No models match your search."
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
