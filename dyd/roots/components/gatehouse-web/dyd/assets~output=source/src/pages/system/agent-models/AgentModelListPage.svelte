<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
  import { createAgentModelListController } from "./agent-model-list-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createAgentModelListController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  const matches = (
    model: { id: string; alias: string; provider: string; model: string },
    search: string,
  ) => {
    const query = search.trim().toLocaleLowerCase()
    return (
      query === "" ||
      `${model.id} ${model.alias} ${model.provider} ${model.model}`
        .toLocaleLowerCase()
        .includes(query)
    )
  }
  $effect(() => {
    void controller.load()
  })
  $effect(() => controller.start())
</script>

<SystemFrame active="agent-models" title="Agent models"
  ><section class="system-page">
    <div class="system-page-heading mb-5">
      <div>
        <p class="eyebrow">System</p>
        <h2 class="title is-3">Agent models</h2>
        <p class="subtitle is-6">
          Models with the same workspace priority are selected randomly.
        </p>
      </div>
      <RouterLink class="button is-primary" href="/app/system/agent-models/new"
        >Add model</RouterLink
      >
    </div>
    <div class="collection-search">
      <label
        ><span>Search agent models</span><input
          class="input"
          type="search"
          autocomplete="off"
          placeholder="Search by ID, alias, provider, or model"
          bind:value={controller.state.search}
        /></label
      >
    </div>
    {#if controller.state.error !== ""}<p
        class="help is-danger"
        aria-live="polite"
      >
        {controller.state.error}
      </p>{/if}
    <div class="system-grant-list">
      {#each controller.state.models.filter( (model) => matches(model, controller.state.search), ) as model (model.id)}<RouterLink
          class={`system-grant-row${model.enabled ? "" : " system-grant-disabled"}`}
          href={`/app/system/agent-models/${encodeURIComponent(model.id)}`}
          ><div>
            <strong>{model.alias}</strong><small
              >{model.id} / {model.provider} / {model.model} / revision {model.revision}</small
            >
          </div>
          <span>{model.enabled ? "Enabled" : "Disabled"}</span></RouterLink
        >{:else}<p class="dashboard-empty">
          No agent models match your search.
        </p>{/each}
    </div>
  </section></SystemFrame
>
