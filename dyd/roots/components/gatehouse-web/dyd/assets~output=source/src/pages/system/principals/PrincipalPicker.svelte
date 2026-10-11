<script lang="ts">
  import * as UI from "../../../ui"
  import {
    searchSystemPrincipals,
    type SystemPrincipal,
  } from "../../../app/system"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { createDirectoryController } from "../../../utils/directory-controller.svelte"
  import { onMount } from "svelte"
  let { value = $bindable("") }: { value?: string } = $props()
  const runtime = useRuntime()
  const controller = createDirectoryController<SystemPrincipal>({
    onAuthenticationLost: runtime.requireLogin,
    onSystemAccessChange: runtime.access.setSystemAccess,
    fetchPage: async (name, cursor, status, signal) => {
      const response = await searchSystemPrincipals(
        name,
        cursor,
        "enabled",
        signal,
      )
      const body = response.ok
        ? ((await response.json()) as {
            principals: SystemPrincipal[]
            next_cursor?: string
          })
        : undefined
      return { response, items: body?.principals, next: body?.next_cursor }
    },
  })
  let selected = $state<SystemPrincipal | undefined>()
  $effect(() => {
    const found = controller.state.items.find(
      (principal) => principal.id === value,
    )
    if (found) selected = found
  })
  onMount(() => {
    return controller.stop
  })
</script>

<UI.RecordPicker
  label="Principal"
  bind:value
  bind:search={controller.state.search}
  selectedLabel={selected?.id === value
    ? (selected.name ?? selected.alias ?? value)
    : undefined}
  items={controller.state.items.map((principal) => ({
    id: principal.id,
    label: principal.name ?? principal.alias ?? principal.id,
    description: principal.alias,
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
