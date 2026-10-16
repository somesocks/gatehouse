<script lang="ts">
  import { onMount } from "svelte"
  import * as UI from "../ui"
  import { createPasswordController } from "./password-controller.svelte"

  const controller = createPasswordController(window.location.hash)
  onMount(() => {
    void controller.load()
    return controller.stop
  })
  $effect(() => {
    if (
      controller.state.status === "success" ||
      controller.state.status === "invalid"
    ) {
      window.history.replaceState(
        null,
        "",
        window.location.pathname + window.location.search,
      )
    }
  })
</script>

<svelte:head>
  <title>Set password · Gatehouse</title>
  <meta name="referrer" content="no-referrer" />
</svelte:head>

<UI.Theme />
<UI.StatusPage
  eyebrow="Gatehouse"
  title={controller.state.status === "success"
    ? "Password updated"
    : "Set your password"}
  description={controller.state.status === "success"
    ? "Your new password is ready to use."
    : controller.state.username
      ? `Choose a new password for ${controller.state.username}.`
      : undefined}
  busy={controller.state.status === "loading"}
>
  {#if controller.state.error}<UI.Notice variant="destructive" role="alert"
      >{controller.state.error}</UI.Notice
    >{/if}
  {#if controller.state.status === "ready"}
    <UI.Form
      onsubmit={(event) => {
        event.preventDefault()
        void controller.save()
      }}
    >
      <UI.Field>
        <UI.Label for="new-password">New password</UI.Label>
        <UI.Input
          id="new-password"
          type="password"
          autocomplete="new-password"
          required
          disabled={controller.state.pending}
          bind:value={controller.state.password}
        />
      </UI.Field>
      <UI.Field>
        <UI.Label for="confirm-password">Confirm password</UI.Label>
        <UI.Input
          id="confirm-password"
          type="password"
          autocomplete="new-password"
          required
          disabled={controller.state.pending}
          bind:value={controller.state.confirmation}
        />
      </UI.Field>
      <UI.Text as="small" tone="muted"
        >Link expires {new Date(
          controller.state.expiresAt,
        ).toLocaleString()}.</UI.Text
      >
      <UI.Button type="submit" disabled={controller.state.pending}
        >{controller.state.pending ? "Saving…" : "Set password"}</UI.Button
      >
    </UI.Form>
  {:else if controller.state.status === "unavailable"}
    <UI.Button onclick={() => void controller.load()}>Try again</UI.Button>
  {:else if controller.state.status === "success"}
    <UI.Link href="/app/login">Sign in</UI.Link>
  {/if}
</UI.StatusPage>
