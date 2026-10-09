<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

  import { createStorageProviderListController } from "./storage-provider-list-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createStorageProviderListController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  const matches = (
    p: { id: string; alias: string; protocol: string; bucket?: string },
    s: string,
  ) => {
    const q = s.trim().toLocaleLowerCase()
    return (
      q === "" ||
      `${p.id} ${p.alias} ${p.protocol} ${p.bucket ?? ""}`
        .toLocaleLowerCase()
        .includes(q)
    )
  }
  $effect(() => {
    void controller.load()
  })
  $effect(() => controller.start())
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "storage-providers")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Storage providers" },
  ]}
  ><UI.Stack as="section">
    <UI.Row justify="between">
      <UI.Group>
        <UI.Text as="p" size="eyebrow">System</UI.Text>
        <UI.Heading level={2}>Storage providers</UI.Heading>
        <UI.Text as="p">Configure embedded and S3 storage.</UI.Text>
      </UI.Group>
      <RouterLink variant="default" href="/app/system/storage-providers/new"
        >Add provider</RouterLink
      >
    </UI.Row>
    <UI.Field>
      <UI.Label
        ><UI.Text>Search storage providers</UI.Text><UI.Input
          type="search"
          bind:value={controller.state.search}
        /></UI.Label
      >
    </UI.Field>
    {#if controller.state.error}<UI.Text as="p" size="small" role="alert">
        {controller.state.error}
      </UI.Text>{/if}
    <UI.List>
      {#each controller.state.providers.filter( (p) => matches(p, controller.state.search), ) as p (p.id)}<RouterLink
          variant="item"
          data-disabled={!p.enabled || undefined}
          href={`/app/system/storage-providers/${encodeURIComponent(p.id)}`}
          ><UI.Stack>
            <UI.Text as="strong" emphasis="strong">{p.alias}</UI.Text><UI.Text
              as="small"
              size="small"
              >{p.id} / {p.protocol} / {p.bucket ?? "no bucket"}</UI.Text
            >
          </UI.Stack>
          <UI.Text>{p.enabled ? "Enabled" : "Disabled"}</UI.Text></RouterLink
        >{:else}<UI.Text as="p" tone="muted">
          No storage providers match your search.
        </UI.Text>{/each}
    </UI.List>
  </UI.Stack></UI.AppFrame
>
