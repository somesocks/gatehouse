<script lang="ts">
  import { untrack } from "svelte"
  import { createAsyncResource } from "../../../utils/load-state.svelte"
  import * as UI from "../../../ui"
  import {
    fetchSystemWorkspace,
    type SystemWorkspace,
  } from "../../../app/system"
  import { systemNavigation } from "../../../app/navigation"
  import { useRuntime } from "../../../app/runtime.svelte"
  import type { Snippet } from "svelte"
  let {
    workspaceID,
    breadcrumbs = [],
    children,
  }: {
    workspaceID: string
    breadcrumbs: { label: string; href?: string }[]
    children: Snippet
  } = $props()
  const runtime = useRuntime()
  const resource = createAsyncResource<SystemWorkspace>()
  const workspace = $derived(resource.value ?? null)
  const error = $derived(resource.lifecycle.state.error)
  async function load() {
    return resource.load(async (signal) => {
      const response = await fetchSystemWorkspace(workspaceID, signal)
      signal.throwIfAborted()
      if (response.status === 401) runtime.requireLogin()
      if (response.status === 403) runtime.access.setSystemAccess("denied")
      if (!response.ok) throw new Error("Workspace could not be loaded.")
      return (await response.json()) as SystemWorkspace
    })
  }
  $effect(() => {
    const id = workspaceID
    untrack(() => {
      resource.context(id)
      void load()
    })
    return resource.stop
  })
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "workspaces")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Workspaces", href: "/app/system/workspaces" },
    {
      label: workspace?.name ?? workspace?.alias ?? "Workspace",
      href: `/app/system/workspaces/${encodeURIComponent(workspaceID)}`,
    },
    ...breadcrumbs,
  ]}
>
  {#if error}<UI.AdminContent
      ><UI.Notice variant="destructive" role="alert"
        >{error}<UI.Button variant="outline" onclick={() => void load()}
          >Retry</UI.Button
        ></UI.Notice
      ></UI.AdminContent
    >{/if}
  {#if resource.lifecycle.reloading}<UI.Text role="status"
      >Refreshing workspace…</UI.Text
    >{/if}
  {#if workspace}{@render children()}{:else if !error}<UI.Text tone="muted"
      >Loading workspace…</UI.Text
    >{/if}
</UI.AppFrame>
