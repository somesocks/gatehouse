<script lang="ts">
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { untrack } from "svelte"
  import { createStorageProviderFormController } from "./storage-provider-form-controller.svelte"
  import StorageProviderFields from "./StorageProviderFields.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createStorageProviderFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    untrack(() => void controller.keychains())
    return controller.stop
  })
  async function create() {
    const route = runtime.state.route
    const provider = await controller.create()
    if (provider && runtime.state.route === route)
      runtime.navigate(
        `/app/system/storage-providers/${encodeURIComponent(provider.id)}`,
        true,
      )
  }
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "storage-providers")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Storage providers", href: "/app/system/storage-providers" },
    { label: "Create" },
  ]}
>
  <UI.AdminContent mode="form"
    ><UI.AdminHeader
      title="Create storage provider"
      description="Configure embedded storage or an S3 connection."
    />
    <UI.Form
      surface
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
    >
      <StorageProviderFields
        bind:form={controller.state.form}
        keychains={controller.state.keychains}
        keychainsLoading={controller.state.keychainsLoading}
        keychainsError={controller.state.keychainsError}
        onKeychainsReload={() => void controller.keychains()}
      />
      {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
          >{controller.state.error}</UI.Notice
        >{/if}
      <UI.FormActions
        ><UI.RouterLink variant="outline" href="/app/system/storage-providers"
          >Cancel</UI.RouterLink
        ><UI.Button type="submit" disabled={controller.state.saving}
          >{controller.state.saving
            ? "Creating…"
            : "Create provider"}</UI.Button
        ></UI.FormActions
      >
    </UI.Form>
  </UI.AdminContent>
</UI.AppFrame>
