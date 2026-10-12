<script lang="ts">
  import { onMount } from "svelte"
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
  let selected = $state<ProjectSummary | null>(null)
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
    const id = value
    const found = controller.state.items.find((item) => item.id === id)
    if (found) selected = found
    else if (id) {
      const request = new AbortController()
      void fetchProject(workspaceID, id, request.signal)
        .then(async (response) => {
          if (response.status === 401) runtime.requireLogin()
          if (response.ok) {
            const project = (await response.json()) as Project
            if (!request.signal.aborted) selected = project
          }
        })
        .catch(() => {})
      return () => request.abort()
    } else selected = null
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
  loading={controller.state.loading}
  error={controller.state.error}
  previous={!!controller.state.history.length}
  next={!!controller.state.next}
  onsearch={() => void controller.search()}
  onprevious={() => void controller.previous()}
  onnext={() => void controller.next()}
  onclose={controller.stop}
/>
