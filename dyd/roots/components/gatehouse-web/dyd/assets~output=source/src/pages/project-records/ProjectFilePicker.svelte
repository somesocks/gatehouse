<script lang="ts">
  import { onMount, untrack } from "svelte"
  import { createAsyncResource } from "../../utils/load-state.svelte"
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
  const selection = createAsyncResource<ProjectFile>()
  const selected = $derived(selection.value ?? null)
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
        const response = await fetchProjectFile(workspace, project, id, signal)
        signal.throwIfAborted()
        if (response.status === 401) runtime.requireLogin()
        if (!response.ok)
          throw new Error("The selected file could not be loaded.")
        return (await response.json()) as ProjectFile
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
  selectedLabel={selected?.id === value ? selected.name : undefined}
  items={controller.state.items.map((file) => ({
    id: file.id,
    label: file.name,
    description: `${formatFileSize(file.size)} · ${file.media_type ?? "Unknown type"}`,
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
