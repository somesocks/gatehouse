<script lang="ts">
  import { untrack } from "svelte"
  import type { Auth } from "../../app/auth.svelte"
  import StatusPage from "../../components/StatusPage.svelte"
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
    <form
      class="stack"
      autocomplete="off"
      onsubmit={(event) => {
        event.preventDefault()
        void controller.submit()
      }}
    >
      <div class="field">
        <label for="identity">Username</label>
        <input
          id="identity"
          name="identity"
          autocomplete="off"
          required
          bind:value={controller.state.identity}
        />
      </div>
      <div class="field">
        <label for="password">Password</label>
        <input
          id="password"
          name="password"
          type="password"
          autocomplete="off"
          required
          bind:value={controller.state.password}
        />
      </div>
      {#if controller.state.error !== ""}
        <p class="field-help" role="alert" aria-live="polite">
          {controller.state.error}
        </p>
      {/if}
      <button class="primary" type="submit" disabled={controller.state.submitting}>
        {controller.state.submitting ? "Signing in..." : "Sign in"}
      </button>
    </form>
  {/snippet}
</StatusPage>
