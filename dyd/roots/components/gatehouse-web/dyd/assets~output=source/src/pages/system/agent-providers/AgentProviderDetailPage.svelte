<script lang="ts">
  import * as UI from "../../../ui"
  import { createConfirmation } from "../../../utils/confirmation.svelte"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { untrack } from "svelte"
  import { createAgentProviderFormController } from "./agent-provider-form-controller.svelte"
  import AgentProviderFields from "./AgentProviderFields.svelte"
  let {
    providerID,
    editingPage = false,
  }: { providerID: string; editingPage?: boolean } = $props()
  const runtime = useRuntime()
  const confirmation = createConfirmation()
  const controller = untrack(() =>
    createAgentProviderFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    const id = providerID
    const edit = editingPage
    let active = true
    const unsubscribe = edit
      ? () => {}
      : runtime.activity.subscribe(
          [
            {
              name: "agent-provider",
              topic: `sys/${id}`,
              events: ["agent_provider.*"],
            },
          ],
          async () => {
            if (!active) return
            await controller.load(id)
            if (active && controller.state.error)
              throw new Error("Provider refresh failed")
          },
        )
    untrack(
      () =>
        void controller.load(id).then(() => {
          if (active && edit) void controller.beginEdit()
        }),
    )
    return () => {
      active = false
      unsubscribe()
      controller.stop()
    }
  })
  async function save() {
    const route = runtime.state.route
    if ((await controller.update()) && runtime.state.route === route)
      runtime.navigate(
        `/app/system/agent-providers/${encodeURIComponent(providerID)}`,
        true,
      )
  }
  const disabling = $derived(controller.state.provider?.enabled && !controller.state.form.enabled)
  function requestSave() {
    if (!disabling) { void save(); return }
    confirmation.request({
      title: "Disable provider?",
      description: `Disable “${controller.state.provider?.alias}”? It will be unavailable for use until enabled again.`,
      confirmLabel: "Disable provider",
      onConfirm: async () => { await save(); return controller.state.error },
    })
  }
  function requestDisconnect() {
    if (!controller.state.provider) return
    const provider = { ...controller.state.provider }
    confirmation.request({
      title: "Disconnect ChatGPT?",
      description: `Remove the stored ChatGPT connection for “${provider.alias}”${provider.account?.email ? ` (${provider.account.email})` : ""}? Reconnect the account to use it again.`,
      confirmLabel: "Disconnect",
      onConfirm: async () => { await controller.disconnect(provider); return controller.state.error },
    })
  }
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "agent-providers")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Agent providers", href: "/app/system/agent-providers" },
    {
      label: controller.state.provider?.alias ?? "Provider",
      href: editingPage
        ? `/app/system/agent-providers/${encodeURIComponent(providerID)}`
        : undefined,
    },
    ...(editingPage ? [{ label: "Edit" }] : []),
  ]}
>
  <UI.AdminContent mode={editingPage ? "form" : "detail"}>
    {#if controller.state.reloading}<UI.Text role="status"
        >Refreshing provider…</UI.Text
      >{/if}
    {#if !editingPage && controller.state.provider && controller.state.error}<UI.Notice
        variant="destructive"
        role="alert">{controller.state.error}</UI.Notice
      >{/if}
    {#if controller.state.error}<UI.Button
        variant="outline"
        onclick={() =>
          void controller.load(providerID).then(() => {
            if (editingPage) void controller.beginEdit()
          })}>Retry loading</UI.Button
      >{/if}
    {#if controller.state.conflict}<UI.Button
        variant="outline"
        onclick={() => void controller.replaceDraft()}
        >Replace draft with latest settings</UI.Button
      >{/if}
    {#if controller.state.loading}<UI.Text tone="muted"
        >Loading provider…</UI.Text
      >
    {:else if !controller.state.provider}<UI.Notice
        variant="destructive"
        role="alert"
        >{controller.state.error || "Provider was not found."}</UI.Notice
      >
    {:else if editingPage}
      <UI.AdminHeader
        title={`Edit ${controller.state.provider.alias}`}
        id={providerID}
        description="Update the provider connection, credentials, and availability."
      />
      <UI.Form
        surface
        onsubmit={(event) => {
          event.preventDefault()
          requestSave()
        }}
      >
        <AgentProviderFields
          bind:form={controller.state.form}
          editing
        />
        {#if controller.state.error}<UI.Notice
            variant="destructive"
            role="alert">{controller.state.error}</UI.Notice
          >{/if}
        <UI.FormActions
          ><UI.RouterLink
            variant="outline"
            href={`/app/system/agent-providers/${encodeURIComponent(providerID)}`}
            >Cancel</UI.RouterLink
          ><UI.Button type="submit" disabled={controller.state.saving}
            >{controller.state.saving ? "Saving…" : "Save changes"}</UI.Button
          ></UI.FormActions
        >
      </UI.Form>
    {:else}
      <UI.AdminHeader
        kind="Agent provider"
        title={controller.state.provider.alias}
        id={providerID}
        enabled={controller.state.provider.enabled}
      >
        {#snippet actions()}<UI.RouterLink
            variant="outline"
            href={`/app/system/agent-providers/${encodeURIComponent(providerID)}/edit`}
            >Edit provider</UI.RouterLink
          >{/snippet}
      </UI.AdminHeader>
      <UI.DetailSection title="Connection"
        ><UI.Property label="Protocol"
          >{controller.state.provider.protocol}</UI.Property
        ><UI.Property label="Endpoint"
          >{controller.state.provider.endpoint ?? "Not configured"}</UI.Property
        ></UI.DetailSection
      >
      <UI.DetailSection title="Credentials"
        ><UI.Property label="Keychain"
          >{controller.state.provider.keychain?.id ??
            "Not configured"}{#if controller.state.provider.keychain}
            · version {controller.state.provider.keychain
              .version}{/if}</UI.Property
        >{#if controller.state.provider.protocol === "openai-codex"}
          <UI.Property label="Status">
            <UI.Row gap="normal">
              <UI.Text
                >{controller.state.provider.credential_configured
                  ? "Connected"
                  : "Not connected"}</UI.Text
              >
              <UI.Link
                variant="default"
                size="sm"
                href={`/app/login/openai-codex?provider=${encodeURIComponent(providerID)}`}
                target="_blank"
                rel="noopener noreferrer"
              >
                {controller.state.provider.credential_configured
                  ? "Reconnect ChatGPT"
                  : "Connect ChatGPT"}
              </UI.Link>
              {#if controller.state.provider.credential_configured}
                <UI.Button
                  variant="outline"
                  size="sm"
                  disabled={controller.state.saving}
                  onclick={requestDisconnect}
                  >{controller.state.saving
                    ? "Disconnecting…"
                    : "Disconnect"}</UI.Button
                >
              {/if}
            </UI.Row>
          </UI.Property>
          {#if controller.state.provider.account}
            <UI.Property label="Account"
              >{controller.state.provider.account.email ??
                controller.state.provider.account.id}</UI.Property
            >
          {/if}
        {:else}<UI.Property label="API key"
            >{controller.state.provider.credential_configured
              ? "Configured"
              : "Not configured"}</UI.Property
          >{/if}</UI.DetailSection
      >
      <UI.DetailSection title="Record information"
        ><UI.Property label="Alias"
          >{controller.state.provider.alias}</UI.Property
        ><UI.Property label="Revision"
          >{controller.state.provider.revision}</UI.Property
        ></UI.DetailSection
      >
    {/if}
  </UI.AdminContent>
</UI.AppFrame>

<UI.ConfirmationDialog {confirmation} contextKey={runtime.state.route} />
