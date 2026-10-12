<script lang="ts">
  import { onMount, untrack } from "svelte"
  import { createAsyncResource } from "../../utils/load-state.svelte"
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
  const selection = createAsyncResource<ProjectRecordSchema>()
  const selected = $derived(selection.value ?? null)
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
  const scope = $derived(`${workspaceID}/${projectID}`)
  $effect(() => {
    scope
    untrack(controller.reset)
  })
  $effect(() => {
    const id = value
    const workspace = workspaceID
    const project = projectID
    const found = controller.state.items.find((item) => item.id === id)
    untrack(() => {
      selection.context(`${workspace}/${project}/${id}`)
      if (!id) return
      void selection.load(async (signal) => {
        if (found) return found
        const response = await fetchProjectRecordSchema(
          workspace,
          project,
          id,
          signal,
        )
        signal.throwIfAborted()
        if (response.status === 401) runtime.requireLogin()
        if (!response.ok)
          throw new Error("The selected record type could not be loaded.")
        return (await response.json()) as ProjectRecordSchema
      })
    })
    return selection.stop
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
  loading={controller.state.busy}
  error={controller.state.error || selection.lifecycle.state.error}
  previous={!!controller.state.history.length}
  next={!!controller.state.next}
  onsearch={() => void controller.search()}
  onprevious={() => void controller.previous()}
  onnext={() => void controller.next()}
  onclose={controller.stop}
/>
