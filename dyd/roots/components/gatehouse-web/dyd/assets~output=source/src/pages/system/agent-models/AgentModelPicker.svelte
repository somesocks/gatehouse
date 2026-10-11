<script lang="ts">
  import * as UI from "../../../ui"
  import {
    searchSystemAgentModels,
    systemAdministration,
    type SystemAgentModel,
  } from "../../../app/system"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import { onMount } from "svelte"
  let { value = $bindable("") }: { value?: string } = $props()
  let selected = $state<SystemAgentModel | null>(null)
  const runtime = useRuntime()
  const controller = createDirectoryController<SystemAgentModel>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await searchSystemAgentModels(name, cursor, "", signal)
      const body = response.ok
        ? ((await response.json()) as {
            models: SystemAgentModel[]
            next_cursor?: string
          })
        : undefined
      return { response, items: body?.models, next: body?.next_cursor }
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
        `agent-models/${encodeURIComponent(id)}`,
        "GET",
        undefined,
        request.signal,
      )
        .then(async (response) => {
          if (response.ok) {
            const result = (await response.json()) as SystemAgentModel
            if (!request.signal.aborted) selected = result
          }
        })
        .catch(() => {})
      return () => request.abort()
    }
  })
</script>

<UI.RecordPicker
  label="Model"
  bind:value
  bind:search={controller.state.search}
  selectedLabel={selected?.id === value ? selected.alias : undefined}
  items={controller.state.items.map((model) => ({
    id: model.id,
    label: model.alias,
    description: model.model,
    disabled: !model.enabled && model.id !== value,
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
