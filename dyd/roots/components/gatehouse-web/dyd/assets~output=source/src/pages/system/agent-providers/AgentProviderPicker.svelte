<script lang="ts">
  import * as UI from "../../../ui"
  import {
    searchSystemAgentProviders,
    systemAdministration,
    type SystemAgentProvider,
  } from "../../../app/system"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import { onMount } from "svelte"
  let { value = $bindable("") }: { value?: string } = $props()
  let selected = $state<SystemAgentProvider | null>(null)
  const runtime = useRuntime()
  const controller = createDirectoryController<SystemAgentProvider>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await searchSystemAgentProviders(
        name,
        cursor,
        "",
        signal,
      )
      const body = response.ok
        ? ((await response.json()) as {
            providers: SystemAgentProvider[]
            next_cursor?: string
          })
        : undefined
      return { response, items: body?.providers, next: body?.next_cursor }
    },
  })
  onMount(() => {
    return controller.stop
  })
  $effect(() => {
    const id = value
    const found = controller.state.items.find((item) => item.id === id)
    if (found) selected = found
    else if (id && selected?.id !== id) {
      const request = new AbortController()
      void systemAdministration(
        `agent-providers/${encodeURIComponent(id)}`,
        "GET",
        undefined,
        request.signal,
      )
        .then(async (response) => {
          if (response.ok) {
            const result = (await response.json()) as SystemAgentProvider
            if (!request.signal.aborted) selected = result
          }
        })
        .catch(() => {})
      return () => request.abort()
    }
  })
</script>

<UI.RecordPicker
  label="Provider"
  bind:value
  bind:search={controller.state.search}
  selectedLabel={selected?.id === value ? selected.alias : undefined}
  items={controller.state.items.map((provider) => ({
    id: provider.id,
    label: provider.alias,
    description: provider.protocol,
    disabled: !provider.enabled && provider.id !== value,
  }))}
  loading={controller.state.loading}
  error={controller.state.error}
  previous={!!controller.state.history.length}
  next={!!controller.state.next}
  onsearch={() => void controller.search()}
  onprevious={() => void controller.previous()}
  onnext={() => void controller.next()}
  onclose={controller.stop}
/>
