<script lang="ts">
  import { signOut } from "../../app/auth"
  import { useRuntime } from "../../app/runtime.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import LoginPage from "../login/LoginPage.svelte"

  const runtime = useRuntime()
  const { access, auth } = runtime
  const route = $derived(runtime.state.route)

  $effect(() => {
    if (route.kind === "login" && auth.state.status === "authenticated" && access.state.workspaceStatus === "ready") {
      runtime.navigate(nextPath(route.next), true)
    }
  })

  function nextPath(next: string | null): string {
    if (next === null) return "/app/"
    let destination: URL
    try {
      destination = new URL(next, window.location.origin)
    } catch {
      return "/app/"
    }
    if (destination.origin !== window.location.origin || !destination.pathname.startsWith("/app/") || destination.pathname === "/app/login" || destination.pathname === "/app/login/") return "/app/"
    return destination.pathname + destination.search + destination.hash
  }

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
  <main class="auth-shell"><section class="status-card"><p class="eyebrow">Gatehouse</p><h1 class="title is-3">No workspace access</h1><p class="subtitle is-6">Ask an administrator to add {auth.state.claims?.principal.name ?? "User"} to a workspace group.</p>{#if access.state.systemAccess === "available"}<RouterLink class="button is-primary is-light is-fullwidth" href="/app/system">System</RouterLink>{/if}<button class="button is-danger is-light is-fullwidth" type="button" onclick={() => void logout()}>Log out</button></section></main>
{/if}
