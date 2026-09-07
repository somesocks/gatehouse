<script lang="ts">
  import type { Snippet } from "svelte"
  import { useRuntime } from "../app/runtime.svelte"
  import LoginPage from "../pages/login/LoginPage.svelte"

  let { children }: { children: Snippet } = $props()
  const runtime = useRuntime()
  const { access, auth } = runtime
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <main class="auth-shell" aria-busy="true" aria-live="polite"><section class="status-card"><p class="eyebrow">Gatehouse</p><div class="loading-mark" aria-hidden="true"></div><p>{auth.state.status === "checking" ? "Checking your session." : "Loading your workspaces."}</p></section></main>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <main class="auth-shell"><section class="status-card"><p class="eyebrow">Gatehouse</p><h1 class="title is-3">Connection unavailable</h1><p class="subtitle is-6">Gatehouse could not load your account.</p><button class="button is-primary" type="button" onclick={() => void runtime.refresh()}>Try again</button></section></main>
{:else if auth.state.status !== "authenticated"}
  <LoginPage {auth} onAuthenticated={() => void runtime.refresh()} />
{:else if access.state.systemAccess === "checking"}
  <section class="system-page"><p class="dashboard-empty">Loading system access...</p></section>
{:else if access.state.systemAccess !== "available"}
  <section class="system-page system-access-denied"><p class="eyebrow">System</p><h2 class="title is-3">System access required</h2><p>You do not currently have an enabled system manager grant.</p></section>
{:else}
  {@render children()}
{/if}
