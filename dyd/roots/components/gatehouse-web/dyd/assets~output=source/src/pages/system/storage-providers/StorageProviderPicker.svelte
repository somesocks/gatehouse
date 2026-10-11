<script lang="ts">
  import * as UI from "../../../ui"
  import {
    searchSystemStorageProviders,
    type SystemStorageProvider,
  } from "../../../app/system"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import { onMount } from "svelte"
  let { value = $bindable("") }: { value?: string } = $props()
  const runtime = useRuntime()
  const controller = createDirectoryController<SystemStorageProvider>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await searchSystemStorageProviders(
        name,
        cursor,
        "enabled",
        signal,
      )
      const body = response.ok
        ? ((await response.json()) as {
            providers: SystemStorageProvider[]
            next_cursor?: string
          })
        : undefined
      return { response, items: body?.providers, next: body?.next_cursor }
    },
  })
  let selected = $state<SystemStorageProvider | undefined>()
  $effect(() => {
    const found = controller.state.items.find(
      (provider) => provider.id === value,
    )
    if (found) selected = found
  })
  onMount(() => {
    return controller.stop
  })
</script>

<UI.RecordPicker
  label="Storage provider"
  bind:value
  bind:search={controller.state.search}
  selectedLabel={selected?.id === value ? selected.alias : undefined}
  items={controller.state.items.map((provider) => ({
    id: provider.id,
    label: provider.alias,
    description: provider.bucket ?? provider.protocol,
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
