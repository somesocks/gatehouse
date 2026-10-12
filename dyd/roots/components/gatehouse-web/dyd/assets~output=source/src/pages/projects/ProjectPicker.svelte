<script lang="ts">
  import { onMount, untrack } from "svelte"
  import { createAsyncResource } from "../../utils/load-state.svelte"
  import * as UI from "../../ui"
  import { useRuntime } from "../../app/runtime.svelte"
  import {
    fetchProject,
    fetchProjects,
    type Project,
    type ProjectSummary,
    type ProjectSearchResponse,
  } from "../../app/projects"
  import { createDirectoryController } from "../../utils/directory-controller.svelte"
  let {
    workspaceID,
    value = $bindable(""),
    compact = false,
    placeholder,
  }: {
    workspaceID: string
    value?: string
    compact?: boolean
    placeholder?: string
  } = $props()
  const runtime = useRuntime()
  const selection = createAsyncResource<ProjectSummary>()
  const selected = $derived(selection.value ?? null)
  const controller = createDirectoryController<ProjectSummary>({
    onAuthenticationLost: runtime.requireLogin,
    fetchPage: async (name, cursor, _status, signal) => {
      const response = await fetchProjects(workspaceID, name, cursor, signal)
      const body = response.ok
        ? ((await response.json()) as ProjectSearchResponse)
        : undefined
      return { response, items: body?.projects, next: body?.next_cursor }
    },
  })
  onMount(() => controller.stop)
  $effect(() => {
    workspaceID
    untrack(controller.reset)
  })
  $effect(() => {
    const id = value
    const workspace = workspaceID
    const found = controller.state.items.find((item) => item.id === id)
    untrack(() => {
      selection.context(`${workspace}/${id}`)
      if (!id) return
      void selection.load(async (signal) => {
        if (found) return found
        const response = await fetchProject(workspace, id, signal)
        signal.throwIfAborted()
        if (response.status === 401) runtime.requireLogin()
        if (!response.ok)
          throw new Error("The selected project could not be loaded.")
        return (await response.json()) as Project
      })
    })
    return selection.stop
  })
</script>

<UI.RecordPicker
  showIdentifiers={false}
  label="Project"
  {compact}
  {placeholder}
  bind:value
  bind:search={controller.state.search}
  selectedLabel={selected?.id === value
    ? (selected.name ?? "Untitled project")
    : undefined}
  items={controller.state.items.map((project) => ({
    id: project.id,
    label: project.name ?? "Untitled project",
    description: project.created_at,
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
