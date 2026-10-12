<script lang="ts">
  import { onMount } from "svelte"
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
  let selected = $state<ProjectRecord | null>(null)
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
  $effect(() => {
    const id = value
    const found = controller.state.items.find((item) => item.id === id)
    if (found) selected = found
    else if (id) {
      const request = new AbortController()
      void fetchProjectRecords(
        workspaceID,
        projectID,
        schemaID,
        request.signal,
        { name: id, limit: 1 },
      )
        .then(async (response) => {
          if (response.status === 401) runtime.requireLogin()
          if (response.ok) {
            const body = (await response.json()) as ProjectRecordsResponse
            if (!request.signal.aborted)
              selected = body.records.find((record) => record.id === id) ?? null
          }
        })
        .catch(() => {})
      return () => request.abort()
    } else selected = null
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
  loading={controller.state.loading}
  error={controller.state.error}
  previous={!!controller.state.history.length}
  next={!!controller.state.next}
  onsearch={() => void controller.search()}
  onprevious={() => void controller.previous()}
  onnext={() => void controller.next()}
  onclose={controller.stop}
/>
