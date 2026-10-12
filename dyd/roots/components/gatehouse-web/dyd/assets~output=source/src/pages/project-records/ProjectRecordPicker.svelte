<script lang="ts">
  import { onMount, untrack } from "svelte"
  import { createAsyncResource } from "../../utils/load-state.svelte"
  import * as UI from "../../ui"
  import { useRuntime } from "../../app/runtime.svelte"
  import {
    fetchProjectRecords,
    type ProjectRecord,
    type ProjectRecordsResponse,
  } from "../../app/project-records"
  import { createDirectoryController } from "../../utils/directory-controller.svelte"
  let {
    workspaceID,
    projectID,
    schemaID,
    value = $bindable(""),
    label = "Record",
  }: {
    workspaceID: string
    projectID: string
    schemaID: string
    value?: string
    label?: string
  } = $props()
  const runtime = useRuntime()
  const selection = createAsyncResource<ProjectRecord>()
  const selected = $derived(selection.value ?? null)
  const display = (record: ProjectRecord) =>
    record.values
      ?.map(
        (item) =>
          item.file?.name ??
          item.reference?.primary_values
            .map((value) => String(value.value))
            .join(" · ") ??
          String(item.value),
      )
      .filter(Boolean)
      .join(" · ") || "Untitled record"
  const controller = createDirectoryController<ProjectRecord>({
    onAuthenticationLost: runtime.requireLogin,
    fetchPage: async (name, cursor, _status, signal) => {
      const response = await fetchProjectRecords(
        workspaceID,
        projectID,
        schemaID,
        signal,
        { name, cursor, limit: 50 },
      )
      const body = response.ok
        ? ((await response.json()) as ProjectRecordsResponse)
        : undefined
      return { response, items: body?.records, next: body?.next_cursor }
    },
  })
  onMount(() => controller.stop)
  const scope = $derived(`${workspaceID}/${projectID}/${schemaID}`)
  $effect(() => {
    scope
    untrack(controller.reset)
  })
  $effect(() => {
    const id = value
    const workspace = workspaceID
    const project = projectID
    const type = schemaID
    const found = controller.state.items.find((item) => item.id === id)
    untrack(() => {
      selection.context(`${workspace}/${project}/${type}/${id}`)
      if (!id) return
      void selection.load(async (signal) => {
        if (found) return found
        const response = await fetchProjectRecords(
          workspace,
          project,
          type,
          signal,
          { name: id, limit: 1 },
        )
        signal.throwIfAborted()
        if (response.status === 401) runtime.requireLogin()
        if (!response.ok)
          throw new Error("The selected record could not be loaded.")
        const body = (await response.json()) as ProjectRecordsResponse
        const record = body.records.find((record) => record.id === id)
        if (!record) throw new Error("The selected record is unavailable.")
        return record
      })
    })
    return selection.stop
  })
</script>

<UI.RecordPicker
  showIdentifiers={false}
  {label}
  bind:value
  bind:search={controller.state.search}
  selectedLabel={selected?.id === value ? display(selected) : undefined}
  items={controller.state.items.map((record) => ({
    id: record.id,
    label: display(record),
    description: record.created_at,
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
