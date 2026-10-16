<script lang="ts">
  import { onDestroy, untrack } from "svelte"
  import * as UI from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { createPrincipalNewController } from "./principal-new-controller.svelte"
  const runtime = useRuntime()
  const controller = untrack(() =>
    createPrincipalNewController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  onDestroy(controller.stop)
  async function create() {
    const route = runtime.state.route
    const principal = await controller.create()
    if (principal && runtime.state.route === route)
      runtime.navigate(
        `/app/system/principals/${encodeURIComponent(principal.id)}`,
        true,
      )
  }
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "principals")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Principals", href: "/app/system/principals" },
    { label: "Create" },
  ]}
>
  <UI.AdminContent mode="form">
    <UI.AdminHeader
      title="Create principal"
      description="Create the account, then add its login identities from the principal page."
    />
    <UI.Form
      surface
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
    >
      <UI.FormSection title="Account details">
        <UI.Field
          ><UI.Label for="principal-name">Display name</UI.Label>
          <UI.Input
            id="principal-name"
            maxlength={256}
            bind:value={controller.state.name}
          />
        </UI.Field>
        <UI.Field
          ><UI.Label for="principal-alias">Alias</UI.Label>
          <UI.Input
            id="principal-alias"
            maxlength={256}
            pattern="[a-z][a-z0-9_-]*"
            bind:value={controller.state.alias}
          />
          <UI.Text as="small" size="small" tone="muted"
            >Optional account alias, separate from login usernames.</UI.Text
          >
        </UI.Field>
      </UI.FormSection>
      <UI.FormSection title="Availability">
        <UI.Label layout="choice"
          ><UI.Checkbox
            bind:checked={controller.state.enabled}
          />Enabled</UI.Label
        >
      </UI.FormSection>
      {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
          >{controller.state.error}</UI.Notice
        >{/if}
      <UI.FormActions>
        <UI.RouterLink variant="outline" href="/app/system/principals"
          >Cancel</UI.RouterLink
        >
        <UI.Button type="submit" disabled={controller.state.saving}
          >{controller.state.saving
            ? "Creating…"
            : "Create principal"}</UI.Button
        >
      </UI.FormActions>
    </UI.Form>
  </UI.AdminContent>
</UI.AppFrame>
