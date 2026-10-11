<script lang="ts">
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { untrack } from "svelte"
  import { createStorageProviderFormController } from "./storage-provider-form-controller.svelte"
  import StorageProviderFields from "./StorageProviderFields.svelte"
  let {
    providerID,
    editingPage = false,
  }: { providerID: string; editingPage?: boolean } = $props()
  const runtime = useRuntime()
  const controller = untrack(() =>
    createStorageProviderFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    const id = providerID
    const edit = editingPage
    void controller.load(id).then(() => {
      if (edit) void controller.beginEdit()
    })
  })
  async function save() {
    const route = runtime.state.route
    if ((await controller.update()) && runtime.state.route === route)
      runtime.navigate(
        `/app/system/storage-providers/${encodeURIComponent(providerID)}`,
        true,
      )
  }
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "storage-providers")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Storage providers", href: "/app/system/storage-providers" },
    {
      label: controller.state.provider?.alias ?? "Provider",
      href: editingPage
        ? `/app/system/storage-providers/${encodeURIComponent(providerID)}`
        : undefined,
    },
    ...(editingPage ? [{ label: "Edit" }] : []),
  ]}
>
  <UI.AdminContent mode={editingPage ? "form" : "detail"}>
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
        description="Update storage settings and availability."
      />
      <UI.Form
        surface
        onsubmit={(event) => {
          event.preventDefault()
          void save()
        }}
      >
        <StorageProviderFields
          bind:form={controller.state.form}
          keychains={controller.state.keychains}
          editing
        />
        {#if controller.state.error}<UI.Notice
            variant="destructive"
            role="alert">{controller.state.error}</UI.Notice
          >{/if}
        <UI.FormActions
          ><UI.RouterLink
            variant="outline"
            href={`/app/system/storage-providers/${encodeURIComponent(providerID)}`}
            >Cancel</UI.RouterLink
          ><UI.Button type="submit" disabled={controller.state.saving}
            >{controller.state.saving ? "Saving…" : "Save changes"}</UI.Button
          ></UI.FormActions
        >
      </UI.Form>
    {:else}
      <UI.AdminHeader
        kind="Storage provider"
        title={controller.state.provider.alias}
        id={providerID}
        enabled={controller.state.provider.enabled}
        >{#snippet actions()}<UI.RouterLink
            variant="outline"
            href={`/app/system/storage-providers/${encodeURIComponent(providerID)}/edit`}
            >Edit provider</UI.RouterLink
          >{/snippet}</UI.AdminHeader
      >
      <UI.DetailSection title="Storage connection">
        <UI.Property label="Protocol"
          >{controller.state.provider.protocol}</UI.Property
        >
        {#if controller.state.provider.protocol === "s3"}<UI.Property
            label="Endpoint"
            >{controller.state.provider.endpoint ??
              "Not configured"}</UI.Property
          ><UI.Property label="Region"
            >{controller.state.provider.region ?? "Not configured"}</UI.Property
          ><UI.Property label="Bucket"
            >{controller.state.provider.bucket ?? "Not configured"}</UI.Property
          >{/if}
      </UI.DetailSection>
      {#if controller.state.provider.protocol === "s3"}<UI.DetailSection
          title="Credentials"
          ><UI.Property label="Access key ID"
            >{controller.state.provider.access_key_id ??
              "Not configured"}</UI.Property
          ><UI.Property label="Keychain"
            >{controller.state.provider.keychain?.id ??
              "Not configured"}</UI.Property
          ><UI.Property label="Secret access key"
            >{controller.state.provider.credential_configured
              ? "Configured"
              : "Not configured"}</UI.Property
          ></UI.DetailSection
        >{/if}
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
