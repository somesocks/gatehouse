<script lang="ts">
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

  import { createSystemGrantsController } from "./system-grants-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() =>
    createSystemGrantsController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
      principalID: () => runtime.auth.state.claims?.principal.ref.id,
    }),
  )
  $effect(() => {
    void controller.load()
  })
  $effect(() => controller.start())
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "grants")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "System grants" },
  ]}
>
  <UI.Stack as="section">
    <UI.Stack>
      <UI.Group>
        <UI.Text as="p" size="eyebrow">System</UI.Text>
        <UI.Heading level={2}>System grants</UI.Heading>
        <UI.Text as="p">
          System managers can modify global Gatehouse state.
        </UI.Text>
      </UI.Group>
    </UI.Stack>
    <UI.Form
      layout="row"
      onsubmit={(event) => {
        event.preventDefault()
        void controller.create()
      }}
    >
      <UI.Label layout="field"
        ><UI.Text>Principal ID</UI.Text><UI.Input
          autocomplete="off"
          placeholder="prn_..."
          bind:value={controller.state.principal}
        /></UI.Label
      ><UI.Button
        variant="default"
        type="submit"
        disabled={controller.state.creating}
        >{controller.state.creating ? "Granting..." : "Add manager"}</UI.Button
      >
    </UI.Form>
    {#if controller.state.error !== ""}<UI.Text
        as="p"
        size="small"
        role="alert"
        aria-live="polite"
      >
        {controller.state.error}
      </UI.Text>{/if}
    <UI.List>
      {#each controller.state.grants as grant (grant.ref.id)}<UI.Item
          data-disabled={!grant.enabled || undefined}
        >
          <UI.Stack>
            <UI.Text as="strong" emphasis="strong">{grant.principal.id}</UI.Text
            ><UI.Text as="small" size="small"
              >{grant.ref.id} / revision {grant.revision}</UI.Text
            >
          </UI.Stack>
          <UI.Row>
            <UI.Text tone={!grant.enabled ? "muted" : "default"}
              >{grant.enabled ? "Enabled" : "Disabled"}</UI.Text
            ><UI.Button
              variant="outline"
              size="sm"
              type="button"
              disabled={controller.state.updatingIDs.has(grant.ref.id)}
              onclick={() => void controller.setEnabled(grant, !grant.enabled)}
              >{controller.state.updatingIDs.has(grant.ref.id)
                ? "Saving..."
                : grant.enabled
                  ? "Disable"
                  : "Enable"}</UI.Button
            >
          </UI.Row>
        </UI.Item>{:else}<UI.Text as="p" tone="muted">
          No system grants are configured.
        </UI.Text>{/each}
    </UI.List>
  </UI.Stack>
</UI.AppFrame>
