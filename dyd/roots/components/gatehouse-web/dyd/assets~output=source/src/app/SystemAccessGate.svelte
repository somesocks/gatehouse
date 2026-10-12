<script lang="ts">
  import * as UI from "../ui"
  import type { Snippet } from "svelte"
  import { useRuntime } from "./runtime.svelte"
  import LoginPage from "../pages/login/LoginPage.svelte"
  import { StatusPage } from "../ui"

  let { children }: { children: Snippet } = $props()
  const runtime = useRuntime()
  const { access, auth } = runtime
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
    {#snippet children()}
      <UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => void runtime.refresh()}>Try again</UI.Button
        ></UI.Group
      >
    {/snippet}
  </StatusPage>
{:else if auth.state.status !== "authenticated"}
  <LoginPage {auth} onAuthenticated={() => void runtime.refresh()} />
{:else if access.state.systemAccess === "checking"}
  <StatusPage busy>
    {#snippet children()}<UI.Text as="p">Loading system access...</UI.Text
      >{/snippet}
  </StatusPage>
{:else if access.state.systemAccess === "unavailable"}
  <StatusPage title="System access could not be loaded"
    ><UI.Button onclick={() => void runtime.refresh()}>Try again</UI.Button
    ></StatusPage
  >
{:else if access.state.systemAccess !== "available"}
  <StatusPage
    eyebrow="System"
    title="System access required"
    description="You do not currently have an enabled system manager grant."
  />
{:else}
  {#if auth.state.reloadError || access.state.reloadError}<UI.Notice
      variant="destructive"
      role="alert"
      >{auth.state.reloadError || access.state.reloadError}<UI.Button
        variant="outline"
        onclick={() => void runtime.refresh()}>Retry</UI.Button
      ></UI.Notice
    >{/if}
  {@render children()}
{/if}
