<script lang="ts">
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    systemAdministration,
    type SystemAgentModel,
  } from "../../../app/system"
  import { untrack } from "svelte"
  import { createWorkspaceAgentBindingController } from "./workspace-agent-binding-controller.svelte"
  import WorkspaceAgentBindingFields from "./WorkspaceAgentBindingFields.svelte"
  import SystemWorkspaceFrame from "../workspaces/SystemWorkspaceFrame.svelte"
  let {
    workspaceID,
    bindingID,
    editingPage = false,
  }: {
    workspaceID: string
    bindingID: string
    editingPage?: boolean
  } = $props()
  const runtime = useRuntime()
  let model = $state<SystemAgentModel | null>(null)
  const controller = untrack(() =>
    createWorkspaceAgentBindingController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    const workspace = workspaceID
    const binding = bindingID
    const editing = editingPage
    let active = true
    untrack(
      () =>
        void controller.load(workspace, binding).then(() => {
          if (active && editing) controller.edit()
        }),
    )
    return () => {
      active = false
      controller.stop()
    }
  })
  $effect(() => {
    const id = controller.state.binding?.model
    model = null
    if (!id) return
    const request = new AbortController()
    void systemAdministration(
      `agent-models/${encodeURIComponent(id)}`,
      "GET",
      undefined,
      request.signal,
    )
      .then(async (response) => {
        if (response.ok) {
          const result = (await response.json()) as SystemAgentModel
          if (!request.signal.aborted) model = result
        }
      })
      .catch(() => {})
    return () => request.abort()
  })
  async function save() {
    const route = runtime.state.route
    await controller.update()
    if (
      !controller.state.editing &&
      !controller.state.error &&
      runtime.state.route === route
    )
      runtime.navigate(
        `/app/system/workspaces/${encodeURIComponent(workspaceID)}/agent-bindings/${encodeURIComponent(bindingID)}`,
        true,
      )
  }
</script>

<SystemWorkspaceFrame
  {workspaceID}
  breadcrumbs={[
    {
      label: "Agent bindings",
      href: `/app/system/workspaces/${encodeURIComponent(workspaceID)}/agent-bindings`,
    },
    {
      label: controller.state.binding?.alias ?? "Binding",
      href: editingPage
        ? `/app/system/workspaces/${encodeURIComponent(workspaceID)}/agent-bindings/${encodeURIComponent(bindingID)}`
        : undefined,
    },
    ...(editingPage ? [{ label: "Edit" }] : []),
  ]}
>
  <UI.AdminContent mode={editingPage ? "form" : "detail"}>
    {#if controller.state.conflict}<UI.Button
        variant="outline"
        onclick={controller.replaceDraft}
        >Replace draft with latest settings</UI.Button
      >{/if}
    {#if controller.state.reloading}<UI.Text role="status"
        >Refreshing binding…</UI.Text
      >{/if}
    {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
        >{controller.state.error}<UI.Button
          variant="outline"
          onclick={() =>
            void controller.load(workspaceID, bindingID).then(() => {
              if (editingPage) controller.edit()
            })}>Retry loading</UI.Button
        ></UI.Notice
      >{/if}
    {#if controller.state.loading}<UI.Text tone="muted"
        >Loading binding…</UI.Text
      >{:else if !controller.state.binding}<UI.Notice
        variant="destructive"
        role="alert"
        >{controller.state.error || "Binding was not found."}</UI.Notice
      >
    {:else if editingPage}<UI.AdminHeader
        title={`Edit ${controller.state.binding.alias}`}
        id={bindingID}
        description="Update this workspace agent’s model and behavior."
      />
      <UI.Form
        surface
        onsubmit={(event) => {
          event.preventDefault()
          void save()
        }}
        ><WorkspaceAgentBindingFields
          bind:form={controller.state.form}
          editing
        />{#if controller.state.error}<UI.Notice
            variant="destructive"
            role="alert">{controller.state.error}</UI.Notice
          >{/if}<UI.FormActions
          ><UI.RouterLink
            variant="outline"
            href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/agent-bindings/${encodeURIComponent(bindingID)}`}
            >Cancel</UI.RouterLink
          ><UI.Button
            type="submit"
            disabled={controller.state.saving || !controller.state.form.model}
            >{controller.state.saving ? "Saving…" : "Save changes"}</UI.Button
          ></UI.FormActions
        ></UI.Form
      >
    {:else}<UI.AdminHeader
        kind="Agent binding"
        title={controller.state.binding.alias}
        id={bindingID}
        enabled={controller.state.binding.enabled}
        >{#snippet actions()}<UI.RouterLink
            variant="outline"
            href={`/app/system/workspaces/${encodeURIComponent(workspaceID)}/agent-bindings/${encodeURIComponent(bindingID)}/edit`}
            >Edit binding</UI.RouterLink
          >{/snippet}</UI.AdminHeader
      >
      <UI.DetailSection title="Model and availability"
        ><UI.Property label="Model"
          ><UI.RouterLink
            href={`/app/system/agent-models/${encodeURIComponent(controller.state.binding.model)}`}
            >{model?.alias ?? controller.state.binding.model}</UI.RouterLink
          ></UI.Property
        ><UI.Property label="Label"
          >{controller.state.binding.label ?? "Not configured"}</UI.Property
        ><UI.Property label="Available to all workspace members"
          >{controller.state.binding.default ? "Yes" : "No"}</UI.Property
        ></UI.DetailSection
      >
      <UI.DetailSection title="Prompt and prelude"
        ><UI.Property label="System prompt"
          ><UI.Text
            >{controller.state.binding.system_prompt ??
              "Not configured"}</UI.Text
          ></UI.Property
        ><UI.Property label="Lisp prelude"
          ><UI.CodeBlock
            value={controller.state.binding.prelude ?? ""}
          /></UI.Property
        ></UI.DetailSection
      >
      <UI.DetailSection title="Token rate limits"
        ><UI.Property label="Limits"
          ><UI.RateLimitsView
            value={controller.state.binding.rate_limits}
          /></UI.Property
        ></UI.DetailSection
      >
      <UI.DetailSection title="Record information"
        ><UI.Property label="Alias"
          >{controller.state.binding.alias}</UI.Property
        ><UI.Property label="Revision"
          >{controller.state.binding.revision}</UI.Property
        ></UI.DetailSection
      >
    {/if}
  </UI.AdminContent>
</SystemWorkspaceFrame>
