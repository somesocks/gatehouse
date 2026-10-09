<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

  import { createWorkspaceStorageBindingsController } from "./workspace-storage-bindings-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createWorkspaceStorageBindingsController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    void controller.load()
  })
  $effect(() => controller.start())
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "workspace-storage-bindings")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Workspace storage bindings" },
  ]}
  ><UI.Stack as="section">
    <UI.Row justify="between">
      <UI.Group>
        <UI.Text as="p" size="eyebrow">System</UI.Text>
        <UI.Heading level={2}>Workspace storage bindings</UI.Heading>
        <UI.Text as="p">Bind global storage providers to workspaces.</UI.Text>
      </UI.Group>
      <RouterLink
        variant="default"
        href="/app/system/workspace-storage-bindings/new"
        >Add binding</RouterLink
      >
    </UI.Row>
    <UI.List>
      {#each controller.state.bindings as b (`${b.workspace}/${b.provider}`)}<RouterLink
          variant="item"
          data-disabled={!b.enabled || undefined}
          href={`/app/system/workspace-storage-bindings/${encodeURIComponent(b.workspace)}/${encodeURIComponent(b.provider)}`}
          ><UI.Stack>
            <UI.Text as="strong" emphasis="strong"
              >{b.workspace} / {b.provider}</UI.Text
            ><UI.Text as="small" size="small"
              >priority {b.priority} / revision {b.revision}</UI.Text
            >
          </UI.Stack>
          <UI.Text>{b.enabled ? "Enabled" : "Disabled"}</UI.Text></RouterLink
        >{:else}<UI.Text as="p" tone="muted">
          No workspace storage bindings are configured.
        </UI.Text>{/each}
    </UI.List>
  </UI.Stack></UI.AppFrame
>
