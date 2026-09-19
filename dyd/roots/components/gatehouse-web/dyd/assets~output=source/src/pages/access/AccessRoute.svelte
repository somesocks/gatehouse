<script lang="ts">
  import { signOut } from "../../app/auth"
  import { useRuntime } from "../../app/runtime.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import StatusPage from "../../components/StatusPage.svelte"
  import LoginPage from "../login/LoginPage.svelte"
  import { loginDestination } from "./login-destination"

  const runtime = useRuntime()
  const { access, auth } = runtime
  const route = $derived(runtime.state.route)

  $effect(() => {
    if (
      route.kind === "login" &&
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "ready"
    ) {
      runtime.navigate(
        loginDestination(route.next, window.location.origin),
        true,
      )
    }
  })

  async function logout(): Promise<void> {
    try {
      await signOut()
    } finally {
      runtime.requireLogin()
    }
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <StatusPage eyebrow="Gatehouse" busy live="polite">
    {#snippet children()}
      <p>
        {auth.state.status === "checking"
          ? "Checking your session."
          : "Loading your workspaces."}
      </p>
    {/snippet}
  </StatusPage>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage
    eyebrow="Gatehouse"
    title="Connection unavailable"
    description="Gatehouse could not load your account."
  >
    {#snippet children()}<div><button class="primary" type="button" onclick={() => void runtime.refresh()}>Try again</button></div>{/snippet}
  </StatusPage>
{:else if auth.state.status !== "authenticated"}
  <LoginPage {auth} onAuthenticated={() => void runtime.refresh()} />
{:else}
  <StatusPage
    eyebrow="Gatehouse"
    title="No workspace access"
    description={`Ask an administrator to add ${auth.state.claims?.principal.name ?? "User"} to a workspace group.`}
  >
    {#snippet children()}
      {#if access.state.systemAccess === "available"}<div><RouterLink class="primary" href="/app/system">System</RouterLink></div>{/if}
      <div><button class="secondary" type="button" onclick={() => void logout()}>Log out</button></div>
    {/snippet}
  </StatusPage>
{/if}
