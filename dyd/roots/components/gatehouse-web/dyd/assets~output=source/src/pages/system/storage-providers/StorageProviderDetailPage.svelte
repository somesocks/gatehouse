<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

  import { createStorageProviderFormController } from "./storage-provider-form-controller.svelte"

  let { providerID }: { providerID: string } = $props()
  const runtime = useRuntime()
  const controller = untrack(() =>
    createStorageProviderFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    void controller.load(providerID)
  })
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "storage-providers")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Storage provider" },
  ]}
>
  <UI.Stack as="section">
    {#if controller.state.loading}
      <UI.Text as="p" tone="muted">Loading storage provider...</UI.Text>
    {:else if controller.state.provider === null}
      <UI.Text as="p" size="small" role="alert">
        {controller.state.error || "Storage provider was not found."}
      </UI.Text>
      <RouterLink variant="outline" href="/app/system/storage-providers"
        >Back to providers</RouterLink
      >
    {:else if !controller.state.editing}
      <UI.Row justify="between">
        <UI.Group>
          <UI.Text as="p" size="eyebrow">Storage provider</UI.Text>
          <UI.Heading level={2}>{controller.state.provider.alias}</UI.Heading>
          <UI.Text as="p">{controller.state.provider.id}</UI.Text>
        </UI.Group>
        <UI.Button
          variant="outline"
          type="button"
          onclick={() => void controller.beginEdit()}>Edit</UI.Button
        >
      </UI.Row>
      <UI.Stack as="dl">
        <UI.Field>
          <UI.Text as="dt" size="small">Protocol</UI.Text>
          <UI.Text as="dd">{controller.state.provider.protocol}</UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Endpoint</UI.Text>
          <UI.Text as="dd"
            >{controller.state.provider.endpoint ?? "Not configured"}</UI.Text
          >
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Region</UI.Text>
          <UI.Text as="dd"
            >{controller.state.provider.region ?? "Not configured"}</UI.Text
          >
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Bucket</UI.Text>
          <UI.Text as="dd"
            >{controller.state.provider.bucket ?? "Not configured"}</UI.Text
          >
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Access key ID</UI.Text>
          <UI.Text as="dd"
            >{controller.state.provider.access_key_id ??
              "Not configured"}</UI.Text
          >
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Keychain</UI.Text>
          <UI.Text as="dd">
            {controller.state.provider.keychain === undefined
              ? "Not configured"
              : `${controller.state.provider.keychain.id} / version ${controller.state.provider.keychain.version}`}
          </UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Credential</UI.Text>
          <UI.Text as="dd">
            {controller.state.provider.credential_configured
              ? "Configured"
              : "Not configured"}
          </UI.Text>
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Status</UI.Text>
          <UI.Text as="dd"
            >{controller.state.provider.enabled
              ? "Enabled"
              : "Disabled"}</UI.Text
          >
        </UI.Field>
      </UI.Stack>
    {:else}
      <UI.Stack>
        <UI.Group>
          <UI.Text as="p" size="eyebrow">Storage provider</UI.Text>
          <UI.Heading level={2}
            >Edit {controller.state.provider.alias}</UI.Heading
          >
          <UI.Text as="p">Update provider settings.</UI.Text>
        </UI.Group>
      </UI.Stack>
      <UI.Form
        onsubmit={(event) => {
          event.preventDefault()
          void controller.update()
        }}
      >
        <UI.Field>
          <UI.Label for="storage-provider-alias">Alias</UI.Label>
          <UI.Group>
            <UI.Input
              id="storage-provider-alias"
              disabled
              value={controller.state.form.alias}
            />
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="storage-provider-protocol">Protocol</UI.Label>
          <UI.Group>
            <UI.Group>
              <UI.Select
                id="storage-provider-protocol"
                bind:value={controller.state.form.protocol}
                ><UI.Option value="embedded">Embedded</UI.Option><UI.Option
                  value="s3">S3</UI.Option
                ></UI.Select
              >
            </UI.Group>
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="storage-provider-endpoint">Endpoint</UI.Label>
          <UI.Group>
            <UI.Input
              id="storage-provider-endpoint"
              bind:value={controller.state.form.endpoint}
            />
          </UI.Group>
        </UI.Field>
        {#if controller.state.form.protocol === "s3"}
          <UI.Field>
            <UI.Label for="storage-provider-region">Region</UI.Label>
            <UI.Group>
              <UI.Input
                id="storage-provider-region"
                bind:value={controller.state.form.region}
              />
            </UI.Group>
          </UI.Field>
          <UI.Field>
            <UI.Label for="storage-provider-bucket">Bucket</UI.Label>
            <UI.Group>
              <UI.Input
                id="storage-provider-bucket"
                bind:value={controller.state.form.bucket}
              />
            </UI.Group>
          </UI.Field>
          <UI.Field>
            <UI.Label for="storage-provider-access-key-id"
              >Access key ID</UI.Label
            >
            <UI.Group>
              <UI.Input
                id="storage-provider-access-key-id"
                bind:value={controller.state.form.accessKeyID}
              />
            </UI.Group>
          </UI.Field>
          <UI.Field>
            <UI.Label for="storage-provider-keychain">Keychain</UI.Label>
            <UI.Group>
              <UI.Group>
                <UI.Select
                  id="storage-provider-keychain"
                  required
                  bind:value={controller.state.form.keychain}
                  ><UI.Option value="">Select keychain</UI.Option
                  >{#each controller.state.keychains as keychain (`${keychain.id}/${keychain.version}`)}<UI.Option
                      value={keychain.id}
                      >{keychain.id} / version {keychain.version}</UI.Option
                    >{/each}</UI.Select
                >
              </UI.Group>
            </UI.Group>
          </UI.Field>
          <UI.Field>
            <UI.Label for="storage-provider-secret"
              >Replacement secret access key</UI.Label
            >
            <UI.Group>
              <UI.Input
                id="storage-provider-secret"
                type="password"
                bind:value={controller.state.form.secret}
              />
            </UI.Group>
            <UI.Text as="p" size="small">
              Leave blank to preserve the current secret. Required when changing
              protocol or keychain.
            </UI.Text>
          </UI.Field>
        {/if}
        <UI.Field>
          <UI.Label layout="choice"
            ><UI.Checkbox bind:checked={controller.state.form.enabled} /> Enabled</UI.Label
          >
        </UI.Field>
        <UI.Row>
          <UI.Group>
            <UI.Button
              variant="default"
              type="submit"
              disabled={controller.state.saving}>Save changes</UI.Button
            >
          </UI.Group>
          <UI.Group>
            <UI.Button
              variant="outline"
              type="button"
              onclick={() => (controller.state.editing = false)}
              >Cancel</UI.Button
            >
          </UI.Group>
        </UI.Row>
      </UI.Form>
      {#if controller.state.error}<UI.Text as="p" size="small" role="alert">
          {controller.state.error}
        </UI.Text>{/if}
    {/if}
  </UI.Stack>
</UI.AppFrame>
