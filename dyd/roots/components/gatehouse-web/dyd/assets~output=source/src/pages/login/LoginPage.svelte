<script lang="ts">
  import { StatusPage } from "../../ui"
  import * as UI from "../../ui"
  import { untrack } from "svelte"
  import type { Auth } from "../../app/auth.svelte"
  import { createLoginController } from "./login-controller.svelte"

  let { auth, onAuthenticated }: { auth: Auth; onAuthenticated: () => void } =
    $props()
  const controller = untrack(() => createLoginController(auth, onAuthenticated))
</script>

<StatusPage
  eyebrow="Gatehouse"
  title="Welcome back."
  description="Sign in to continue to your workspace."
>
  {#snippet children()}
    <UI.Form
      autocomplete="off"
      onsubmit={(event) => {
        event.preventDefault()
        void controller.submit()
      }}
    >
      <UI.Field>
        <UI.Label for="identity">Username</UI.Label>
        <UI.Input
          id="identity"
          name="identity"
          autocomplete="off"
          required
          bind:value={controller.state.identity}
        />
      </UI.Field>
      <UI.Field>
        <UI.Label for="password">Password</UI.Label>
        <UI.Input
          id="password"
          name="password"
          type="password"
          autocomplete="off"
          required
          bind:value={controller.state.password}
        />
      </UI.Field>
      {#if controller.state.error !== ""}
        <UI.Text as="p" size="small" role="alert" aria-live="polite">
          {controller.state.error}
        </UI.Text>
      {/if}
      <UI.Button
        variant="default"
        type="submit"
        disabled={controller.state.submitting}
      >
        {controller.state.submitting ? "Signing in..." : "Sign in"}
      </UI.Button>
    </UI.Form>
  {/snippet}
</StatusPage>
