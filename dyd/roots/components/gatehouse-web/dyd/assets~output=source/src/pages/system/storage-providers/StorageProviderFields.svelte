<script lang="ts">
  import * as UI from "../../../ui"
  import type { createStorageProviderFormController } from "./storage-provider-form-controller.svelte"
  import type { SystemKeychain } from "../../../app/system"
  import KeychainPicker from "../keychains/KeychainPicker.svelte"
  let {
    form = $bindable(),
    keychains,
    editing = false,
  }: {
    form: ReturnType<
      typeof createStorageProviderFormController
    >["state"]["form"]
    keychains: SystemKeychain[]
    editing?: boolean
  } = $props()
</script>

{#if !editing}<UI.FormSection title="Identity"
    ><UI.Field
      ><UI.Label for="storage-alias">Alias</UI.Label><UI.Input
        id="storage-alias"
        required
        pattern="[a-z][a-z0-9_-]*"
        bind:value={form.alias}
      /></UI.Field
    ></UI.FormSection
  >{/if}
<UI.FormSection title="Storage connection">
  <UI.Field
    ><UI.Label for="storage-protocol">Protocol</UI.Label><UI.Select
      id="storage-protocol"
      bind:value={form.protocol}
      ><UI.Option value="embedded">Embedded</UI.Option><UI.Option value="s3"
        >S3</UI.Option
      ></UI.Select
    ></UI.Field
  >
  {#if form.protocol === "s3"}
    <UI.Field
      ><UI.Label for="storage-endpoint">Endpoint</UI.Label><UI.Input
        id="storage-endpoint"
        type="url"
        required
        bind:value={form.endpoint}
      /></UI.Field
    >
    <UI.Grid columns={2}
      ><UI.Field
        ><UI.Label for="storage-region">Region</UI.Label><UI.Input
          id="storage-region"
          required
          bind:value={form.region}
        /></UI.Field
      ><UI.Field
        ><UI.Label for="storage-bucket">Bucket</UI.Label><UI.Input
          id="storage-bucket"
          required
          bind:value={form.bucket}
        /></UI.Field
      ></UI.Grid
    >
  {/if}
</UI.FormSection>
{#if form.protocol === "s3"}<UI.FormSection
    title="Credentials"
    description="Credentials are write-only. Existing secrets are never displayed."
  >
    <UI.Field
      ><UI.Label for="storage-access">Access key ID</UI.Label><UI.Input
        id="storage-access"
        required
        bind:value={form.accessKeyID}
      /></UI.Field
    >
    <KeychainPicker bind:value={form.keychain} {keychains} />
    <UI.Field
      ><UI.Label for="storage-secret"
        >{editing
          ? "Replacement secret access key"
          : "Secret access key"}</UI.Label
      ><UI.Input
        id="storage-secret"
        type="password"
        autocomplete="new-password"
        required={!editing}
        bind:value={form.secret}
      />{#if editing}<UI.Text as="small" size="small" tone="muted"
          >Leave blank to keep the current secret. Replace it when changing
          protocol or keychain.</UI.Text
        >{/if}</UI.Field
    >
  </UI.FormSection>{/if}
<UI.FormSection title="Availability"
  ><UI.Label layout="choice"
    ><UI.Checkbox bind:checked={form.enabled} />Enabled</UI.Label
  ></UI.FormSection
>
