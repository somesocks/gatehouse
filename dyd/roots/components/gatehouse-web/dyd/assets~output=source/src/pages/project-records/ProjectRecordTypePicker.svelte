<script lang="ts">
  import { onMount } from "svelte"
  import * as UI from "../../ui"
  import { useRuntime } from "../../app/runtime.svelte"
  import {
    fetchProjectRecordSchema,
    fetchProjectRecordSchemas,
    type ProjectRecordSchema,
  } from "../../app/project-records"
  import { createDirectoryController } from "../../utils/directory-controller.svelte"
  let {
    workspaceID,
    projectID,
    value = $bindable(""),
  }: { workspaceID: string; projectID: string; value?: string } = $props()
  const runtime = useRuntime()
  let selected = $state<ProjectRecordSchema | null>(null)
  const controller = createDirectoryController<ProjectRecordSchema>({
    onAuthenticationLost: runtime.requireLogin,
    fetchPage: async (name, cursor, _status, signal) => {
      const response = await fetchProjectRecordSchemas(
        workspaceID,
        projectID,
        signal,
        { name, cursor, limit: 50 },
      )
      return {
        response,
        items: response.ok
          ? ((await response.json()) as ProjectRecordSchema[])
          : undefined,
        next: response.headers.get("X-Next-Cursor") ?? undefined,
      }
    },
  })
  onMount(() => controller.stop)
  $effect(() => {
    const id = value
    const found = controller.state.items.find((item) => item.id === id)
    if (found) selected = found
    else if (id) {
      const request = new AbortController()
      void fetchProjectRecordSchema(workspaceID, projectID, id, request.signal)
        .then(async (response) => {
          if (response.status === 401) runtime.requireLogin()
          if (response.ok) {
            const schema = (await response.json()) as ProjectRecordSchema
            if (!request.signal.aborted) selected = schema
          }
        })
        .catch(() => {})
      return () => request.abort()
    } else selected = null
  })
</script>

<UI.RecordPicker
  showIdentifiers={false}
  label="Target record type"
  bind:value
  bind:search={controller.state.search}
  selectedLabel={selected?.id === value ? selected.label : undefined}
  items={controller.state.items.map((schema) => ({
    id: schema.id,
    label: schema.label,
    description: schema.description || schema.name,
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
