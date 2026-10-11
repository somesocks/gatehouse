<script lang="ts">
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
  let workspace = $state<SystemWorkspace | null>(null)
  let error = $state("")
  $effect(() => {
    const id = workspaceID
    const request = new AbortController()
    workspace = null
    error = ""
    void fetchSystemWorkspace(id, request.signal)
      .then(async (response) => {
        if (request.signal.aborted) return
        if (response.status === 401) runtime.requireLogin()
        else if (response.status === 403)
          runtime.access.setSystemAccess("denied")
        else if (!response.ok) error = "Workspace could not be loaded."
        else {
          const result = (await response.json()) as SystemWorkspace
          if (!request.signal.aborted) workspace = result
        }
      })
      .catch(() => {
        if (!request.signal.aborted) error = "Workspace could not be loaded."
      })
    return () => request.abort()
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
      ><UI.Notice variant="destructive" role="alert">{error}</UI.Notice
      ></UI.AdminContent
    >{:else if workspace}{@render children()}{:else}<UI.Text tone="muted"
      >Loading workspace…</UI.Text
    >{/if}
</UI.AppFrame>
