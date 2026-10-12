<script lang="ts">
  import { onMount } from "svelte"
  import * as UI from "../../ui"
  import { useRuntime } from "../../app/runtime.svelte"
  import {
    fetchProjectFile,
    fetchProjectFiles,
    type ProjectFile,
  } from "../../app/project-files"
  import { createDirectoryController } from "../../utils/directory-controller.svelte"
  import { formatFileSize } from "../../utils/file-size"
  let {
    workspaceID,
    projectID,
    value = $bindable(""),
    label = "Project file",
  }: {
    workspaceID: string
    projectID: string
    value?: string
    label?: string
  } = $props()
  const runtime = useRuntime()
  let selected = $state<ProjectFile | null>(null)
  const controller = createDirectoryController<ProjectFile>({
    onAuthenticationLost: runtime.requireLogin,
    fetchPage: async (name, cursor, _status, signal) => {
      const response = await fetchProjectFiles(workspaceID, projectID, signal, {
        name,
        cursor,
        limit: 50,
      })
      return {
        response,
        items: response.ok
          ? ((await response.json()) as ProjectFile[])
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
      void fetchProjectFile(workspaceID, projectID, id, request.signal)
        .then(async (response) => {
          if (response.status === 401) runtime.requireLogin()
          if (response.ok) {
            const file = (await response.json()) as ProjectFile
            if (!request.signal.aborted) selected = file
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
  selectedLabel={selected?.id === value ? selected.name : undefined}
  items={controller.state.items.map((file) => ({
    id: file.id,
    label: file.name,
    description: `${formatFileSize(file.size)} · ${file.media_type ?? "Unknown type"}`,
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
