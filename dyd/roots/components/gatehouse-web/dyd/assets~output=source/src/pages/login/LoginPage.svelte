<script lang="ts">
  import { untrack } from "svelte"
  import type { Auth } from "../../app/auth.svelte"
  import { createLoginController } from "./login-controller.svelte"

  let { auth, onAuthenticated }: { auth: Auth; onAuthenticated: () => void } =
    $props()
  const controller = untrack(() => createLoginController(auth, onAuthenticated))
</script>

<main class="status-page">
  <section class="login-card">
    <p class="eyebrow">Gatehouse</p>
    <h1 class="title is-2">Welcome back.</h1>
    <p class="subtitle is-6">Sign in to continue to your workspace.</p>
    <form
      autocomplete="off"
      onsubmit={(event) => {
        event.preventDefault()
        void controller.submit()
      }}
    >
      <div class="field">
        <label class="label" for="identity">Username</label>
        <div class="control">
          <input
            class="input"
            id="identity"
            name="identity"
            autocomplete="off"
            required
            bind:value={controller.state.identity}
          />
        </div>
      </div>
      <div class="field">
        <label class="label" for="password">Password</label>
        <div class="control">
          <input
            class="input"
            id="password"
            name="password"
            type="password"
            autocomplete="off"
            required
            bind:value={controller.state.password}
          />
        </div>
      </div>
      {#if controller.state.error !== ""}
        <p class="help is-danger" aria-live="polite">
          {controller.state.error}
        </p>
      {/if}
      <div class="field login-action">
        <div class="control">
          <button
            class="button is-primary is-fullwidth"
            type="submit"
            disabled={controller.state.submitting}
          >
            {controller.state.submitting ? "Signing in..." : "Sign in"}
          </button>
        </div>
      </div>
    </form>
  </section>
</main>
