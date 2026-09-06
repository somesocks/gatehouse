<script lang="ts">
  import { signOut } from "../../app/auth"
  import { useRuntime } from "../../app/runtime.svelte"
  import type { Route } from "../../route"
  import LoginPage from "../login/LoginPage.svelte"
  import SystemPage from "./SystemPage.svelte"

  type SystemRoute = Extract<Route, { kind: "system" | "system-grants" | "system-principals" }>

  const runtime = useRuntime()
  const { access, activity, auth } = runtime
  let mobileMenuOpen = $state(false)
  const currentRoute = $derived(runtime.state.route as SystemRoute)

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
  <SystemPage route={currentRoute} systemAccess={access.state.systemAccess} {activity} principalID={() => auth.state.claims?.principal.ref.id} principalName={auth.state.claims?.principal.name ?? "User"} onAuthenticationLost={() => runtime.requireLogin()} onSystemAccessChange={access.setSystemAccess} onLogout={() => void logout()} bind:mobileMenuOpen />
{/if}
