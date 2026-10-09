<script lang="ts">
  import { RouterLink, StatusPage } from "../../ui"
  import * as UI from "../../ui"
  import { signOut } from "../../app/auth"
  import { useRuntime } from "../../app/runtime.svelte"
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
      <UI.Text as="p">
        {auth.state.status === "checking"
          ? "Checking your session."
          : "Loading your workspaces."}
      </UI.Text>
    {/snippet}
  </StatusPage>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage
    eyebrow="Gatehouse"
    title="Connection unavailable"
    description="Gatehouse could not load your account."
  >
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => void runtime.refresh()}>Try again</UI.Button
        ></UI.Group
      >{/snippet}
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
      {#if access.state.systemAccess === "available"}<UI.Group
          ><RouterLink variant="default" href="/app/system">System</RouterLink
          ></UI.Group
        >{/if}
      <UI.Group
        ><UI.Button
          variant="outline"
          type="button"
          onclick={() => void logout()}>Log out</UI.Button
        ></UI.Group
      >
    {/snippet}
  </StatusPage>
{/if}
