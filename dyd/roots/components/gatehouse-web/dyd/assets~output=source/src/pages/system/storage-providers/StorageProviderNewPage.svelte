<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

  import { createStorageProviderFormController } from "./storage-provider-form-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() =>
    createStorageProviderFormController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    void controller.keychains()
  })

  async function create(): Promise<void> {
    const provider = await controller.create()
    if (provider)
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
    { label: "New storage provider" },
  ]}
>
  <UI.Stack as="section">
    <UI.Stack>
      <UI.Group>
        <UI.Text as="p" size="eyebrow">System</UI.Text>
        <UI.Heading level={2}>New storage provider</UI.Heading>
        <UI.Text as="p">
          Credentials are write-only and cannot be viewed after saving.
        </UI.Text>
      </UI.Group>
    </UI.Stack>
    <UI.Form
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
    >
      <UI.Field>
        <UI.Label for="storage-provider-alias">Alias</UI.Label>
        <UI.Group>
          <UI.Input
            id="storage-provider-alias"
            required
            bind:value={controller.state.form.alias}
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
          <UI.Label for="storage-provider-access-key-id">Access key ID</UI.Label
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
          <UI.Label for="storage-provider-secret">Secret access key</UI.Label>
          <UI.Group>
            <UI.Input
              id="storage-provider-secret"
              type="password"
              required
              bind:value={controller.state.form.secret}
            />
          </UI.Group>
        </UI.Field>
      {/if}
      <UI.Row>
        <UI.Group>
          <UI.Button
            variant="default"
            type="submit"
            disabled={controller.state.saving}>Add provider</UI.Button
          >
        </UI.Group>
        <UI.Group>
          <RouterLink variant="outline" href="/app/system/storage-providers"
            >Cancel</RouterLink
          >
        </UI.Group>
      </UI.Row>
    </UI.Form>
    {#if controller.state.error}<UI.Text as="p" size="small" role="alert">
        {controller.state.error}
      </UI.Text>{/if}
  </UI.Stack>
</UI.AppFrame>
