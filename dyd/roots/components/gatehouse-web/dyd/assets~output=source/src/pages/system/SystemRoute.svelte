<script lang="ts">
  import { signOut } from "../../app/auth"
  import { useRuntime } from "../../app/runtime.svelte"
  import LoginPage from "../login/LoginPage.svelte"
  import SystemPage from "./SystemPage.svelte"

  const runtime = useRuntime()
  const { access, auth } = runtime
  let mobileMenuOpen = $state(false)

  async function logout(): Promise<void> {
    try {
      await signOut()
    } finally {
      runtime.requireLogin()
    }
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <main class="auth-shell" aria-busy="true" aria-live="polite"><section class="status-card"><p class="eyebrow">Gatehouse</p><div class="loading-mark" aria-hidden="true"></div><p>{auth.state.status === "checking" ? "Checking your session." : "Loading your workspaces."}</p></section></main>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <main class="auth-shell"><section class="status-card"><p class="eyebrow">Gatehouse</p><h1 class="title is-3">Connection unavailable</h1><p class="subtitle is-6">Gatehouse could not load your account.</p><button class="button is-primary" type="button" onclick={() => void runtime.refresh()}>Try again</button></section></main>
{:else if auth.state.status !== "authenticated"}
  <LoginPage {auth} onAuthenticated={() => void runtime.refresh()} />
{:else}
  <SystemPage systemAccess={access.state.systemAccess} principalName={auth.state.claims?.principal.name ?? "User"} onLogout={() => void logout()} bind:mobileMenuOpen />
{/if}
