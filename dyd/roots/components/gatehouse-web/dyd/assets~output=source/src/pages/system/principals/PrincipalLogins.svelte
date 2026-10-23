<script lang="ts">
  import { untrack } from "svelte"
  import * as UI from "../../../ui"
  import { createConfirmation } from "../../../utils/confirmation.svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import type { SystemPrincipal, SystemPrincipalIdentity } from "../../../app/system"
  import { createPrincipalLoginsController } from "./principal-logins-controller.svelte"
  let {
    principal,
    onChanged,
    onReload,
  }: {
    principal: SystemPrincipal
    onChanged: (principal: SystemPrincipal) => void
    onReload: () => Promise<void>
  } = $props()
  const runtime = useRuntime()
  const confirmation = createConfirmation()
  const principalID = $derived(principal.id)
  const controller = createPrincipalLoginsController({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    onChanged: (next) => onChanged(next),
    currentIdentityID: () => runtime.auth.state.claims?.identity,
    onReload: () => onReload(),
  })
  const disabling = $derived(controller.state.mode === "edit" && controller.state.identity?.enabled && !controller.state.enabled)
  $effect(() => {
    const id = principalID
    untrack(() => controller.reset(id))
    return controller.stop
  })
  function saveLogin() {
    if (!disabling) { void controller.save(); return }
    confirmation.request({
      title: "Disable login?",
      description: `Disable ${controller.state.identity?.key}? It will no longer be usable for sign-in. This may sign you out if it is your current identity.`,
      confirmLabel: "Disable login",
      onConfirm: async () => { await controller.save(); return controller.state.error },
    })
  }
  function setEnabled(identity: SystemPrincipalIdentity) {
    if (!identity.enabled) { void controller.setEnabled(identity); return }
    const snapshot = { ...identity }
    confirmation.request({
      title: "Disable login?",
      description: `Disable ${identity.key}? It will no longer be usable for sign-in. This may sign you out if it is your current identity.`,
      confirmLabel: "Disable login",
      onConfirm: async () => { await controller.setEnabled(snapshot); return controller.state.error },
    })
  }
  function remove(identity: SystemPrincipalIdentity) {
    const snapshot = { ...identity }
    confirmation.request({
      title: "Delete login?",
      description: `Delete ${identity.key} and its password? This may sign you out if it is your current identity.`,
      confirmLabel: "Delete login",
      onConfirm: async () => { await controller.remove(snapshot); return controller.state.error },
    })
  }
</script>

<UI.Stack>
  <UI.Row
    ><UI.Heading level={3}>Login identities</UI.Heading>
    <UI.Button
      variant="outline"
      disabled={controller.state.saving}
      onclick={() => controller.begin("add")}>Add username/password</UI.Button
    >
  </UI.Row>
  {#if controller.state.notice}<UI.Text role="status"
      >{controller.state.notice}</UI.Text
    >{/if}
  {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
      >{controller.state.error}</UI.Notice
    >{/if}
  {#if controller.state.passwordLink}
    <UI.FormSection
      title={`Set-password link for ${controller.state.passwordLink.username}`}
    >
      <UI.Field>
        <UI.Label for="set-password-link">Share this link</UI.Label>
        <UI.Input
          id="set-password-link"
          readonly
          value={controller.state.passwordLink.url}
        />
      </UI.Field>
      <UI.Text as="small" tone="muted"
        >Expires {new Date(
          controller.state.passwordLink.expires_at,
        ).toLocaleString()}. It can be used once, and login changes invalidate
        it.</UI.Text
      >
      <UI.Row>
        <UI.Button
          variant="outline"
          onclick={() => void controller.copyPasswordLink()}
          >Copy link</UI.Button
        >
        <UI.Button variant="outline" onclick={controller.cancel}
          >Dismiss</UI.Button
        >
      </UI.Row>
    </UI.FormSection>
  {/if}
  {#if controller.state.mode}
    <UI.Form
      surface
      onsubmit={(event) => {
        event.preventDefault()
        saveLogin()
      }}
    >
      <UI.FormSection
        title={controller.state.mode === "add"
          ? "Add username/password"
          : controller.state.mode === "password"
            ? `Set password for ${controller.state.identity?.key}`
            : "Edit login"}
      >
        {#if controller.state.mode !== "password"}
          <UI.Field
            ><UI.Label for="login-username">Username</UI.Label>
            <UI.Input
              id="login-username"
              required
              maxlength={256}
              autocomplete="off"
              bind:value={controller.state.username}
            />
            <UI.Text as="small" size="small" tone="muted"
              >Case-sensitive. No whitespace or colons.</UI.Text
            >
          </UI.Field>
          <UI.Label layout="choice"
            ><UI.Checkbox
              bind:checked={controller.state.enabled}
            />Enabled</UI.Label
          >
        {/if}
        {#if controller.state.mode !== "edit"}
          <UI.Field
            ><UI.Label for="login-password">New password</UI.Label>
            <UI.Input
              id="login-password"
              type="password"
              required
              autocomplete="new-password"
              bind:value={controller.state.password}
            />
          </UI.Field>
          <UI.Field
            ><UI.Label for="login-password-confirmation"
              >Confirm password</UI.Label
            >
            <UI.Input
              id="login-password-confirmation"
              type="password"
              required
              autocomplete="new-password"
              bind:value={controller.state.confirmation}
            />
          </UI.Field>
        {/if}
      </UI.FormSection>
      <UI.FormActions>
        <UI.Button
          type="button"
          variant="outline"
          disabled={controller.state.saving}
          onclick={controller.cancel}>Cancel</UI.Button
        >
        <UI.Button
          type="submit"
          disabled={controller.state.saving || controller.state.conflict}
          >{controller.state.saving ? "Saving…" : "Save login"}</UI.Button
        >
      </UI.FormActions>
    </UI.Form>
  {/if}
  <UI.RecordTable headers={["Identity", "Status", "Revision", "Actions"]}>
    {#each principal.identities as identity (identity.id)}
      <UI.RecordRow>
        <UI.RecordCell primary
          ><UI.Text emphasis="strong">{identity.key}</UI.Text><UI.Identifier
            value={identity.id}
          /></UI.RecordCell
        >
        <UI.RecordCell label="Status"
          ><UI.StatusBadge enabled={identity.enabled} /></UI.RecordCell
        >
        <UI.RecordCell label="Revision">{identity.revision}</UI.RecordCell>
        <UI.RecordCell label="Actions"
          ><UI.Row>
            {#if identity.key.startsWith("gatehouse:")}
              <UI.Button
                variant="outline"
                disabled={controller.state.saving}
                onclick={() => controller.begin("edit", identity)}
                >Edit username</UI.Button
              >
              <UI.Button
                variant="outline"
                disabled={controller.state.saving}
                onclick={() => controller.begin("password", identity)}
                >Set password</UI.Button
              >
              <UI.Button
                variant="outline"
                disabled={controller.state.saving ||
                  !principal.enabled ||
                  !identity.enabled}
                onclick={() => void controller.createPasswordLink(identity)}
                >Create set-password link</UI.Button
              >
              <UI.Button
                variant="outline"
                disabled={controller.state.saving}
                onclick={() => setEnabled(identity)}
                >{identity.enabled ? "Disable" : "Enable"}</UI.Button
              >
            {/if}
            <UI.Button
              variant="outline"
              disabled={controller.state.saving}
              onclick={() => remove(identity)}>Delete</UI.Button
            >
          </UI.Row></UI.RecordCell
        >
      </UI.RecordRow>
    {:else}<UI.TableEmpty
        columns={4}
        message="No login identities. Add a username/password to allow this principal to sign in."
      />{/each}
  </UI.RecordTable>
</UI.Stack>

<UI.ConfirmationDialog {confirmation} contextKey={runtime.state.route} />
