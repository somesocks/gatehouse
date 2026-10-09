<script lang="ts">
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

  import { createPrincipalsController } from "./principals-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() =>
    createPrincipalsController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
      principalID: () => runtime.auth.state.claims?.principal.ref.id,
    }),
  )
  $effect(() => {
    void controller.load()
  })
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "principals")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Principals" },
  ]}
>
  <UI.Stack as="section">
    <UI.Stack>
      <UI.Group>
        <UI.Text as="p" size="eyebrow">System</UI.Text>
        <UI.Heading level={2}>Principals</UI.Heading>
        <UI.Text as="p">
          Identity associations are shown without credential verifiers.
        </UI.Text>
      </UI.Group>
    </UI.Stack>
    {#if controller.state.error !== ""}<UI.Text
        as="p"
        size="small"
        role="alert"
        aria-live="polite"
      >
        {controller.state.error}
      </UI.Text>{/if}
    <UI.List>
      {#each controller.state.principals as principal (principal.id)}<UI.Item
          data-disabled={!principal.enabled || undefined}
        >
          <UI.Stack>
            <UI.Text as="strong" emphasis="strong"
              >{principal.name ?? principal.alias ?? principal.id}</UI.Text
            ><UI.Text as="small" size="small"
              >{principal.id}{principal.alias === undefined
                ? ""
                : ` / ${principal.alias}`} / revision {principal.revision}</UI.Text
            >{#if principal.identities.length > 0}<UI.Stack tone="muted">
                {#each principal.identities as identity (identity.id)}<UI.Text
                    tone={!identity.enabled ? "muted" : "default"}
                    >{identity.key} / {identity.id} / revision {identity.revision}{identity.enabled
                      ? ""
                      : " / Disabled"}</UI.Text
                  >{/each}
              </UI.Stack>{:else}<UI.Text as="small" size="small"
                >No identities</UI.Text
              >{/if}
          </UI.Stack>
          <UI.Row>
            <UI.Text tone={!principal.enabled ? "muted" : "default"}
              >{principal.enabled ? "Enabled" : "Disabled"}</UI.Text
            ><UI.Button
              variant="outline"
              size="sm"
              type="button"
              disabled={controller.state.updatingIDs.has(principal.id)}
              onclick={() =>
                void controller.setEnabled(principal, !principal.enabled)}
              >{controller.state.updatingIDs.has(principal.id)
                ? "Saving..."
                : principal.enabled
                  ? "Disable"
                  : "Enable"}</UI.Button
            >
          </UI.Row>
        </UI.Item>{:else}<UI.Text as="p" tone="muted">
          No principals are configured.
        </UI.Text>{/each}
    </UI.List>
  </UI.Stack>
</UI.AppFrame>
