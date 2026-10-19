<script lang="ts">
  import { onMount } from "svelte"
  import * as UI from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  import {
    fetchSystemWorkspaceAgents,
    type SystemWorkspaceAgent,
    type SystemWorkspaceAgentSearchResponse,
  } from "../../../app/system"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"

  let {
    workspaceID,
    value = $bindable(""),
  }: { workspaceID: string; value?: string } = $props()
  const runtime = useRuntime()
  const controller = createDirectoryController<SystemWorkspaceAgent>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    fetchPage: async (name, cursor, _status, signal) => {
      const response = await fetchSystemWorkspaceAgents(
        workspaceID,
        name,
        cursor,
        signal,
      )
      const body = response.ok
        ? ((await response.json()) as SystemWorkspaceAgentSearchResponse)
        : undefined
      return { response, items: body?.agents, next: body?.next_cursor }
    },
  })
  let selected = $state<SystemWorkspaceAgent | undefined>()
  $effect(() => {
    const found = controller.state.items.find((agent) => agent.id === value)
    if (found) selected = found
  })
  onMount(() => controller.stop)
</script>

<UI.RecordPicker
  label="Bot"
  bind:value
  bind:search={controller.state.search}
  selectedLabel={selected?.id === value
    ? (selected.label ?? selected.alias)
    : undefined}
  items={controller.state.items.map((agent) => ({
    id: agent.id,
    label: agent.label ?? agent.alias,
    description: agent.alias,
    disabled: !agent.enabled,
  }))}
  loading={controller.state.busy}
  error={controller.state.error}
  previous={!!controller.state.history.length}
  next={!!controller.state.next}
  onsearch={() => void controller.search()}
  onprevious={() => void controller.previous()}
  onnext={() => void controller.next()}
  onclose={controller.stop}
/>
