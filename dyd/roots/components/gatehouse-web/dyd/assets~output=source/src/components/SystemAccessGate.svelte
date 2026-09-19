<script lang="ts">
  import type { Snippet } from "svelte"
  import { useRuntime } from "../app/runtime.svelte"
  import LoginPage from "../pages/login/LoginPage.svelte"
  import StatusPage from "./StatusPage.svelte"

  let { children }: { children: Snippet } = $props()
  const runtime = useRuntime()
  const { access, auth } = runtime
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
    {#snippet children()}
      <div><button class="primary" type="button" onclick={() => void runtime.refresh()}>Try again</button></div>
    {/snippet}
  </StatusPage>
{:else if auth.state.status !== "authenticated"}
  <LoginPage {auth} onAuthenticated={() => void runtime.refresh()} />
{:else if access.state.systemAccess === "checking"}
  <StatusPage busy>
    {#snippet children()}<p>Loading system access...</p>{/snippet}
  </StatusPage>
{:else if access.state.systemAccess !== "available"}
  <StatusPage
    eyebrow="System"
    title="System access required"
    description="You do not currently have an enabled system manager grant."
  />
{:else}
  {@render children()}
{/if}
