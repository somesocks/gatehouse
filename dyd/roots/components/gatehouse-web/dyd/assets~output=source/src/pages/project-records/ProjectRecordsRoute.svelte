<script lang="ts">
  import { Menu } from "@lucide/svelte"
  import { fetchProject, type Project } from "../../app/projects"

  import {
    fetchProjectFiles,
    projectFileDownloadPath,
    type ProjectFile,
  } from "../../app/project-files"
  import {
    createProjectRecord,
    createProjectRecordAttribute,
    createProjectRecordSchema,
    fetchProjectRecord,
    fetchProjectRecordAttributes,
    fetchProjectRecordIncomingReferences,
    fetchProjectRecordSchema,
    fetchProjectRecordSchemas,
    fetchProjectRecords,
    fetchProjectRecordValues,
    mutateProjectRecordValues,
    removeProjectRecord,
    removeProjectRecordAttribute,
    removeProjectRecordSchema,
    updateProjectRecordAttribute,
    updateProjectRecordSchema,
    type ProjectRecord,
    type ProjectRecordAttribute,
    type ProjectRecordIncomingReferenceGroup,
    type ProjectRecordSchema,
    type ProjectRecordValue,
  } from "../../app/project-records"
  import { useRuntime } from "../../app/runtime.svelte"
  import PageBody from "../../components/PageBody.svelte"
  import PageHeading from "../../components/PageHeading.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import * as SidebarPage from "../../components/sidebar-page"
  import WorkspaceNavigation from "../../components/WorkspaceNavigation.svelte"
  import type { Route } from "../../route"

  type RecordsRoute = Extract<
    Route,
    {
      kind:
        | "project-records"
        | "project-record-schema-new"
        | "project-record-schema"
        | "project-record-schema-edit"
        | "project-record-new"
        | "project-record"
        | "project-record-edit"
    }
  >
  type Status = "checking" | "ready" | "unavailable"
  type ValueDraft = {
    id?: string
    attribute: string
    value: unknown
    sensitive: boolean
    removed?: boolean
  }
  type AttributeForm = {
    id?: string
    name: string
    label: string
    description: string
    type: ProjectRecordAttribute["type"]
    target_schema: string
    cardinality: ProjectRecordAttribute["cardinality"]
    uniqueness: ProjectRecordAttribute["uniqueness"]
    display: ProjectRecordAttribute["display"]
    display_order: number
  }

  const runtime = useRuntime()
  const { activity, access, auth } = runtime
  let project = $state<Project | null>(null)
  let projectStatus = $state<Status>("checking")
  let schemas = $state<ProjectRecordSchema[]>([])
  let schemasStatus = $state<Status>("checking")
  let schema = $state<ProjectRecordSchema | null>(null)
  let attributes = $state<ProjectRecordAttribute[]>([])
  let projectFiles = $state<ProjectFile[]>([])
  let records = $state<ProjectRecord[]>([])
  let recordsStatus = $state<Status>("ready")
  let active = $state<ProjectRecord | null>(null)
  let activeValues = $state<ProjectRecordValue[]>([])
  let incomingReferences = $state<ProjectRecordIncomingReferenceGroup[]>([])
  let incomingReferencesNextCursor = $state<string | undefined>(undefined)
  let incomingReferencesLoading = $state(false)
  let drafts = $state<ValueDraft[]>([])
  let saving = $state(false)
  let error = $state("")
  let schemaName = $state("")
  let schemaLabel = $state("")
  let schemaDescription = $state("")
  let attributeForm = $state<AttributeForm>({
    name: "",
    label: "",
    description: "",
    type: "text",
    target_schema: "",
    cardinality: "one",
    uniqueness: "none",
    display: "none",
    display_order: 0,
  })
  let generation = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as RecordsRoute)
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const projectPath = (workspaceID: string, projectID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}/prj/${encodeURIComponent(projectID)}`
  const recordsPath = (workspaceID: string, projectID: string) =>
    `${projectPath(workspaceID, projectID)}/records`
  const schemaPath = (
    workspaceID: string,
    projectID: string,
    schemaID: string,
  ) => `${recordsPath(workspaceID, projectID)}/${encodeURIComponent(schemaID)}`
  const recordPath = (
    workspaceID: string,
    projectID: string,
    schemaID: string,
    recordID: string,
  ) =>
    `${schemaPath(workspaceID, projectID, schemaID)}/${encodeURIComponent(recordID)}`
  const isSchemaRoute = (
    route: RecordsRoute,
  ): route is Extract<RecordsRoute, { schemaID: string }> => "schemaID" in route
  const isCurrent = (value: number, route: RecordsRoute) =>
    value === generation &&
    currentRoute.workspaceID === route.workspaceID &&
    currentRoute.projectID === route.projectID &&
    !abortController?.signal.aborted
  const valueLabel = (value: unknown) =>
    typeof value === "boolean" ? (value ? "Yes" : "No") : String(value)
  const displayAttributes = $derived(
    attributes.filter((attribute) => attribute.display !== "none"),
  )

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })

  function activate(route: RecordsRoute): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    project = null
    projectStatus = "checking"
    schemas = []
    schemasStatus = "checking"
    schema = null
    attributes = []
    projectFiles = []
    records = []
    recordsStatus =
      route.kind === "project-record-schema" ||
      route.kind === "project-record" ||
      route.kind === "project-record-edit"
        ? "checking"
        : "ready"
    active = null
    activeValues = []
    incomingReferences = []
    incomingReferencesNextCursor = undefined
    incomingReferencesLoading = false
    drafts = []
    saving = false
    error = ""
    schemaName = ""
    schemaLabel = ""
    schemaDescription = ""
    resetAttributeForm()
    if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "ready"
    )
      void loadRoute(route, value, abortController.signal)
    else if (auth.state.status === "authenticated") void runtime.refresh()
    return () => {
      if (value === generation) {
        abortController?.abort()
        unsubscribe?.()
        unsubscribe = undefined
      }
    }
  }

  async function loadRoute(
    route: RecordsRoute,
    value: number,
    signal: AbortSignal,
  ): Promise<void> {
    if (
      !access.state.workspaces.some(
        (candidate) => candidate.id === route.workspaceID,
      )
    ) {
      runtime.navigate(
        access.state.workspaces.length === 0
          ? "/app/no-access"
          : `/app/wsp/${encodeURIComponent(access.state.workspaces[0].id)}`,
        true,
      )
      return
    }
    try {
      const response = await fetchProject(
        route.workspaceID,
        route.projectID,
        signal,
      )
      if (!isCurrent(value, route) || signal.aborted) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (response.status === 404) {
        runtime.navigate(
          `/app/wsp/${encodeURIComponent(route.workspaceID)}/prj`,
          true,
        )
        return
      }
      if (!response.ok) throw new Error()
      project = (await response.json()) as Project
      projectStatus = "ready"
      if (!(await loadSchemas(route, value, signal))) return
      subscribe(route, value)
      if (route.kind === "project-record-schema-new") return
      if (!isSchemaRoute(route)) return
      if (!(await loadSchema(route, value, signal))) return
      if (route.kind === "project-record-schema-edit") {
        schemaLabel = schema?.label ?? ""
        schemaDescription = schema?.description ?? ""
        return
      }
      if (route.kind === "project-record-new") {
        drafts = attributes.map(emptyDraft)
        return
      }
      if (
        route.kind === "project-record" ||
        route.kind === "project-record-edit"
      ) {
        await loadRecord(route, value, signal)
        return
      }
      await loadRecords(route, value, signal)
    } catch {
      if (isCurrent(value, route) && !signal.aborted)
        projectStatus = "unavailable"
    }
  }

  function subscribe(route: RecordsRoute, value: number): void {
    unsubscribe = activity.subscribe(
      [
        {
          name: "project-records",
          topic: `${route.workspaceID}/${route.projectID}`,
          events: [
            "project_record_schema.*",
            "project_record_attribute.*",
            "project_record.*",
            "project_file.*",
          ],
        },
      ],
      async ({ signal }) => {
        if (!isCurrent(value, route) || signal.aborted) return
        if (!(await loadSchemas(route, value, signal)))
          throw new Error("project records refresh failed")
        if (isSchemaRoute(route) && !(await loadSchema(route, value, signal)))
          throw new Error("project record type refresh failed")
        if (
          route.kind === "project-record" ||
          route.kind === "project-record-edit"
        ) {
          if (!(await loadRecord(route, value, signal)))
            throw new Error("project record refresh failed")
        } else if (route.kind === "project-record-schema") {
          if (!(await loadRecords(route, value, signal)))
            throw new Error("project record list refresh failed")
        }
      },
    )
    void activity.poll()
  }

  async function loadSchemas(
    route: RecordsRoute,
    value: number,
    signal: AbortSignal,
  ): Promise<boolean> {
    schemasStatus = "checking"
    try {
      const response = await fetchProjectRecordSchemas(
        route.workspaceID,
        route.projectID,
        signal,
      )
      if (!isCurrent(value, route) || signal.aborted) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error()
      schemas = (await response.json()) as ProjectRecordSchema[]
      schemasStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, route) && !signal.aborted)
        schemasStatus = "unavailable"
      return false
    }
  }

  async function loadSchema(
    route: Extract<RecordsRoute, { schemaID: string }>,
    value: number,
    signal: AbortSignal,
  ): Promise<boolean> {
    try {
      const [schemaResponse, attributesResponse, filesResponse] = await Promise.all([
        fetchProjectRecordSchema(
          route.workspaceID,
          route.projectID,
          route.schemaID,
          signal,
        ),
        fetchProjectRecordAttributes(
          route.workspaceID,
          route.projectID,
          route.schemaID,
          signal,
        ),
        fetchProjectFiles(route.workspaceID, route.projectID, signal),
      ])
      if (!isCurrent(value, route) || signal.aborted) return false
      if (
        schemaResponse.status === 401 ||
        attributesResponse.status === 401 ||
        filesResponse.status === 401
      ) {
        runtime.requireLogin()
        return false
      }
      if (schemaResponse.status === 404) {
        runtime.navigate(recordsPath(route.workspaceID, route.projectID), true)
        return false
      }
      if (!schemaResponse.ok || !attributesResponse.ok || !filesResponse.ok)
        throw new Error()
      schema = (await schemaResponse.json()) as ProjectRecordSchema
      attributes = (await attributesResponse.json()) as ProjectRecordAttribute[]
      projectFiles = (await filesResponse.json()) as ProjectFile[]
      return true
    } catch {
      error = "The record type could not be loaded."
      return false
    }
  }

  async function loadRecords(
    route: Extract<RecordsRoute, { kind: "project-record-schema" }>,
    value: number,
    signal: AbortSignal,
  ): Promise<boolean> {
    recordsStatus = "checking"
    try {
      const response = await fetchProjectRecords(
        route.workspaceID,
        route.projectID,
        route.schemaID,
        signal,
      )
      if (!isCurrent(value, route) || signal.aborted) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error()
      records = ((await response.json()) as { records: ProjectRecord[] })
        .records
      recordsStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, route) && !signal.aborted)
        recordsStatus = "unavailable"
      return false
    }
  }

  async function loadRecord(
    route: Extract<RecordsRoute, { recordID: string }>,
    value: number,
    signal: AbortSignal,
  ): Promise<boolean> {
    recordsStatus = "checking"
    try {
      const [recordResponse, valuesResponse, referencesResponse] =
        await Promise.all([
          fetchProjectRecord(
            route.workspaceID,
            route.projectID,
            route.schemaID,
            route.recordID,
            signal,
          ),
          fetchProjectRecordValues(
            route.workspaceID,
            route.projectID,
            route.schemaID,
            route.recordID,
            signal,
          ),
          fetchProjectRecordIncomingReferences(
            route.workspaceID,
            route.projectID,
            route.schemaID,
            route.recordID,
            undefined,
            signal,
          ),
        ])
      if (!isCurrent(value, route) || signal.aborted) return false
      if (
        recordResponse.status === 401 ||
        valuesResponse.status === 401 ||
        referencesResponse.status === 401
      ) {
        runtime.requireLogin()
        return false
      }
      if (recordResponse.status === 404) {
        runtime.navigate(
          schemaPath(route.workspaceID, route.projectID, route.schemaID),
          true,
        )
        return false
      }
      if (!recordResponse.ok || !valuesResponse.ok || !referencesResponse.ok)
        throw new Error()
      active = (await recordResponse.json()) as ProjectRecord
      activeValues = (
        (await valuesResponse.json()) as { values: ProjectRecordValue[] }
      ).values
      const references = (await referencesResponse.json()) as {
        groups: ProjectRecordIncomingReferenceGroup[]
        next_cursor?: string
      }
      incomingReferences = references.groups
      incomingReferencesNextCursor = references.next_cursor
      drafts = activeValues.map((item) => ({
        id: item.id,
        attribute: item.attribute,
        value: item.value,
        sensitive: item.sensitive,
      }))
      recordsStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, route) && !signal.aborted)
        recordsStatus = "unavailable"
      return false
    }
  }

  function emptyDraft(attribute: ProjectRecordAttribute): ValueDraft {
    return {
      attribute: attribute.id,
      value: attribute.type === "boolean" ? false : "",
      sensitive: false,
    }
  }
  function resetAttributeForm(): void {
    attributeForm = {
      name: "",
      label: "",
      description: "",
      type: "text",
      target_schema: "",
      cardinality: "one",
      uniqueness: "none",
      display: "none",
      display_order: 0,
    }
  }
  function attributeFor(id: string) {
    return attributes.find((attribute) => attribute.id === id)
  }
  function attributeValues(id: string) {
    return drafts.filter((draft) => draft.attribute === id && !draft.removed)
  }
  function editAttribute(attribute: ProjectRecordAttribute): void {
    attributeForm = {
      id: attribute.id,
      name: attribute.name,
      label: attribute.label,
      description: attribute.description,
      type: attribute.type,
      target_schema: attribute.target_schema ?? "",
      cardinality: attribute.cardinality,
      uniqueness: attribute.uniqueness,
      display: attribute.display,
      display_order: attribute.display_order,
    }
  }
  function setDraft(index: number, patch: Partial<ValueDraft>): void {
    drafts = drafts.map((draft, position) =>
      position === index ? { ...draft, ...patch } : draft,
    )
  }
  function draftIndex(draft: ValueDraft): number {
    return drafts.indexOf(draft)
  }
  function typedValue(attribute: ProjectRecordAttribute, raw: string): unknown {
    return attribute.type === "number" ? Number(raw) : raw
  }
  function valueLines(
    attribute: ProjectRecordAttribute,
    value: ProjectRecordValue,
  ): string[] {
    if (attribute.type === "file")
      return [value.file?.name ?? "Unavailable file"]
    if (attribute.type !== "record") return [valueLabel(value.value)]
    if (
      value.reference !== undefined &&
      value.reference.primary_values.length !== 0
    )
      return value.reference.primary_values.map((primary) =>
        valueLabel(primary.value),
      )
    return [
      `Anonymous ${value.reference?.schema_label ?? schemas.find((schema) => schema.id === attribute.target_schema)?.label ?? "Record"}`,
    ]
  }
  function referencePath(
    attribute: ProjectRecordAttribute,
    value: ProjectRecordValue,
  ): string | undefined {
    if (attribute.type === "file" && typeof value.value === "string")
      return projectFileDownloadPath(
        workspace.id,
        currentRoute.projectID,
        value.value,
      )
    return attribute.type === "record" &&
      attribute.target_schema !== undefined &&
      typeof value.value === "string"
      ? recordPath(
          workspace.id,
          currentRoute.projectID,
          attribute.target_schema,
          value.value,
        )
      : undefined
  }
  function primaryLines(recordValues: ProjectRecordValue[]): string[] {
    const attribute = attributes.find(
        (candidate) =>
          candidate.display === "primary" &&
          candidate.type !== "record" &&
          recordValues.some((item) => item.attribute === candidate.id),
    )
    if (attribute === undefined)
      return [`Anonymous ${schema?.label ?? "Record"}`]
    const value = recordValues.find((item) => item.attribute === attribute.id)
    return value === undefined
      ? [`Anonymous ${schema?.label ?? "Record"}`]
      : valueLines(attribute, value)
  }
  function incomingReferenceLabel(
    group: ProjectRecordIncomingReferenceGroup,
    reference: ProjectRecordIncomingReferenceGroup["references"][number],
  ): string {
    const values = reference.primary_values.map((primary) =>
      valueLabel(primary.value),
    )
    return values.length === 0
      ? `Anonymous ${group.source_schema.label}`
      : values.join(" · ")
  }
  function appendIncomingReferences(
    groups: ProjectRecordIncomingReferenceGroup[],
  ): void {
    incomingReferences = groups.reduce((result, group) => {
      const current = result.find(
        (candidate) =>
          candidate.source_schema.id === group.source_schema.id &&
          candidate.source_attribute.id === group.source_attribute.id,
      )
      if (current === undefined)
        return [...result, { ...group, references: [...group.references] }]
      return result.map((candidate) =>
        candidate === current
          ? {
              ...candidate,
              references: [...candidate.references, ...group.references],
            }
          : candidate,
      )
    }, incomingReferences)
  }
  async function loadMoreIncomingReferences(): Promise<void> {
    const route = currentRoute
    const value = generation
    if (
      route.kind !== "project-record" ||
      incomingReferencesNextCursor === undefined ||
      incomingReferencesLoading
    )
      return
    incomingReferencesLoading = true
    try {
      const response = await fetchProjectRecordIncomingReferences(
        route.workspaceID,
        route.projectID,
        route.schemaID,
        route.recordID,
        incomingReferencesNextCursor,
        abortController?.signal,
      )
      if (!isCurrent(value, route)) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error()
      const next = (await response.json()) as {
        groups: ProjectRecordIncomingReferenceGroup[]
        next_cursor?: string
      }
      appendIncomingReferences(next.groups)
      incomingReferencesNextCursor = next.next_cursor
    } catch {
      error = "More reverse references could not be loaded."
    } finally {
      if (isCurrent(value, route)) incomingReferencesLoading = false
    }
  }

  async function saveSchema(): Promise<void> {
    const route = currentRoute
    const value = generation
    saving = true
    error = ""
    try {
      const response =
        route.kind === "project-record-schema-new"
          ? await createProjectRecordSchema(
              route.workspaceID,
              route.projectID,
              {
                name: schemaName,
                label: schemaLabel,
                description: schemaDescription,
              },
              abortController?.signal,
            )
          : isSchemaRoute(route)
            ? await updateProjectRecordSchema(
                route.workspaceID,
                route.projectID,
                route.schemaID,
                { label: schemaLabel, description: schemaDescription },
                abortController?.signal,
              )
            : undefined
      if (response === undefined || !isCurrent(value, route)) return
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error()
      const saved = (await response.json()) as ProjectRecordSchema
      runtime.navigate(
        `${schemaPath(route.workspaceID, route.projectID, saved.id)}/edit`,
      )
    } catch {
      if (isCurrent(value, route))
        error = "The record type could not be saved. Check its name and label."
    } finally {
      if (isCurrent(value, route)) saving = false
    }
  }

  async function deleteSchema(): Promise<void> {
    const route = currentRoute
    if (
      !isSchemaRoute(route) ||
      !window.confirm(`Remove ${schema?.label ?? "this record type"}?`)
    )
      return
    saving = true
    error = ""
    try {
      const response = await removeProjectRecordSchema(
        route.workspaceID,
        route.projectID,
        route.schemaID,
        abortController?.signal,
      )
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error()
      runtime.navigate(recordsPath(route.workspaceID, route.projectID))
    } catch {
      error =
        "The record type could not be removed. Records may still reference it."
    } finally {
      saving = false
    }
  }

  async function saveAttribute(): Promise<void> {
    const route = currentRoute
    if (!isSchemaRoute(route)) return
    saving = true
    error = ""
    const { id, name, target_schema, display_order, ...input } = attributeForm
    try {
      const response =
        id === undefined
          ? await createProjectRecordAttribute(
              route.workspaceID,
              route.projectID,
              route.schemaID,
              {
                name,
                ...input,
                display_order: Number(display_order),
                ...(input.type === "record" ? { target_schema } : {}),
              },
              abortController?.signal,
            )
          : await updateProjectRecordAttribute(
              route.workspaceID,
              route.projectID,
              route.schemaID,
              id,
              {
                ...input,
                display_order: Number(display_order),
                ...(input.type === "record" ? { target_schema } : {}),
              },
              abortController?.signal,
            )
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok) throw new Error()
      attributes = await fetchAttributes(route)
      resetAttributeForm()
    } catch {
      error =
        "The field could not be saved. Field names must be lowercase snake case."
    } finally {
      saving = false
    }
  }

  async function fetchAttributes(
    route: Extract<RecordsRoute, { schemaID: string }>,
  ): Promise<ProjectRecordAttribute[]> {
    const response = await fetchProjectRecordAttributes(
      route.workspaceID,
      route.projectID,
      route.schemaID,
      abortController?.signal,
    )
    if (!response.ok) throw new Error()
    return (await response.json()) as ProjectRecordAttribute[]
  }
  async function deleteAttribute(
    attribute: ProjectRecordAttribute,
  ): Promise<void> {
    const route = currentRoute
    if (!isSchemaRoute(route) || !window.confirm(`Remove ${attribute.label}?`))
      return
    saving = true
    try {
      const response = await removeProjectRecordAttribute(
        route.workspaceID,
        route.projectID,
        route.schemaID,
        attribute.id,
        abortController?.signal,
      )
      if (!response.ok) throw new Error()
      attributes = attributes.filter(
        (candidate) => candidate.id !== attribute.id,
      )
      resetAttributeForm()
    } catch {
      error = "The field could not be removed. It may have values."
    } finally {
      saving = false
    }
  }

  async function saveRecord(): Promise<void> {
    const route = currentRoute
    if (!isSchemaRoute(route)) return
    const value = generation
    saving = true
    error = ""
    try {
      if (route.kind === "project-record-new") {
        const response = await createProjectRecord(
          route.workspaceID,
          route.projectID,
          route.schemaID,
          drafts
            .filter((draft) => !draft.removed)
            .map(({ attribute, value, sensitive }) => ({
              attribute,
              value,
              sensitive,
            })),
          abortController?.signal,
        )
        if (response.status === 401) {
          runtime.requireLogin()
          return
        }
        if (!response.ok) throw new Error()
        const created = (await response.json()) as { record: ProjectRecord }
        runtime.navigate(
          recordPath(
            route.workspaceID,
            route.projectID,
            route.schemaID,
            created.record.id,
          ),
        )
        return
      }
      if (route.kind !== "project-record-edit") return
      const response = await mutateProjectRecordValues(
        route.workspaceID,
        route.projectID,
        route.schemaID,
        route.recordID,
        {
          create: drafts
            .filter((draft) => draft.id === undefined && !draft.removed)
            .map(({ attribute, value, sensitive }) => ({
              attribute,
              value,
              sensitive,
            })),
          update: drafts
            .filter((draft) => draft.id !== undefined && !draft.removed)
            .map(({ id, value, sensitive }) => ({ id: id!, value, sensitive })),
          delete: drafts
            .filter((draft) => draft.id !== undefined && draft.removed)
            .map(({ id }) => ({ id: id! })),
        },
        abortController?.signal,
      )
      if (response.status === 401) {
        runtime.requireLogin()
        return
      }
      if (!response.ok && response.status !== 404) throw new Error()
      runtime.navigate(
        recordPath(
          route.workspaceID,
          route.projectID,
          route.schemaID,
          route.recordID,
        ),
      )
    } catch {
      if (isCurrent(value, route))
        error =
          "The record could not be saved. Check the values and uniqueness rules."
    } finally {
      if (isCurrent(value, route)) saving = false
    }
  }

  async function deleteRecord(): Promise<void> {
    const route = currentRoute
    if (
      route.kind !== "project-record-edit" ||
      !window.confirm("Remove this record?")
    )
      return
    saving = true
    try {
      const response = await removeProjectRecord(
        route.workspaceID,
        route.projectID,
        route.schemaID,
        route.recordID,
        abortController?.signal,
      )
      if (!response.ok) throw new Error()
      runtime.navigate(schemaPath(route.workspaceID, route.projectID))
    } catch {
      error = "The record could not be removed."
    } finally {
      saving = false
    }
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <main class="status-page" aria-busy="true">
    <section class="status-card">
      <div class="loading-mark" aria-hidden="true"></div>
      <p>Loading your workspaces.</p>
    </section>
  </main>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <main class="status-page">
    <section class="status-card">
      <h1 class="title is-3">Connection unavailable</h1>
      <button
        class="button is-primary"
        type="button"
        onclick={() => void runtime.refresh()}>Try again</button
      >
    </section>
  </main>
{:else if auth.state.status !== "authenticated"}
  <main class="status-page">
    <section class="status-card">
      <h1 class="title is-3">Sign in required</h1>
      <button
        class="button is-primary"
        type="button"
        onclick={() => runtime.requireLogin()}>Sign in</button
      >
    </section>
  </main>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <main class="status-page">
    <section class="status-card">
      <h1 class="title is-3">No workspace access</h1>
    </section>
  </main>
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar><WorkspaceNavigation {workspace} active="projects" /></SidebarPage.Sidebar>
    <SidebarPage.Page>
      <SidebarPage.Header>
        <SidebarPage.Toggle><button class="mobile-menu-trigger" type="button" aria-label="Open navigation menu"><Menu size={20} aria-hidden="true" /></button></SidebarPage.Toggle>
        <h1 class="brand-workspace-breadcrumb">
        <RouterLink
          class="brand-workspace-breadcrumb-segment"
          href={`/app/wsp/${encodeURIComponent(workspace.id)}`}
          >{workspace.name ?? workspace.id}</RouterLink
        ><span class="brand-workspace-breadcrumb-separator">/</span><RouterLink
          href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj`}
          >Projects</RouterLink
        ><span class="brand-workspace-breadcrumb-separator">/</span><RouterLink
          class="brand-workspace-breadcrumb-segment"
          href={projectPath(workspace.id, currentRoute.projectID)}
          >{project?.name ?? "Project"}</RouterLink
        ><span class="brand-workspace-breadcrumb-separator">/</span><span
          >Records</span
        >
        </h1>
      </SidebarPage.Header>
      <SidebarPage.Body><PageBody>
      {#if projectStatus === "checking"}<p class="dashboard-empty">
          Loading project...
        </p>
      {:else if projectStatus === "unavailable"}<p class="dashboard-empty">
          Project unavailable.
        </p>
        <button
          class="button is-primary"
          type="button"
          onclick={() =>
            void loadRoute(currentRoute, generation, abortController!.signal)}
          >Try again</button
        >
      {:else if currentRoute.kind === "project-record-schema-new" || currentRoute.kind === "project-record-schema-edit"}<form
          class="record-schema-editor"
          onsubmit={(event) => {
            event.preventDefault()
            void saveSchema()
          }}
        >
          {#snippet schemaActions()}{#if currentRoute.kind === "project-record-schema-edit"}<button
                class="button is-danger is-light"
                type="button"
                disabled={saving}
                onclick={() => void deleteSchema()}>Remove record type</button
              >{/if}{/snippet}
          <PageHeading actions={schemaActions}>
            <p class="eyebrow">Record type</p>
            <h2>
              {currentRoute.kind === "project-record-schema-new"
                ? "New record type"
                : `Edit ${schema?.label ?? "record type"}`}
            </h2>
          </PageHeading>
          <div class="field">
            <label class="label" for="schema-name">Name</label>
            <div class="control">
              <input
                class="input"
                id="schema-name"
                required
                disabled={currentRoute.kind !== "project-record-schema-new"}
                maxlength="64"
                bind:value={schemaName}
                placeholder="contacts"
              />
            </div>
            <p class="help">
              Lowercase letters, numbers, and underscores only.
            </p>
          </div>
          <div class="field">
            <label class="label" for="schema-label">Label</label>
            <div class="control">
              <input
                class="input"
                id="schema-label"
                required
                maxlength="256"
                bind:value={schemaLabel}
              />
            </div>
          </div>
          <div class="field">
            <label class="label" for="schema-description">Description</label>
            <div class="control">
              <textarea
                class="textarea"
                id="schema-description"
                rows="3"
                maxlength="4096"
                bind:value={schemaDescription}></textarea>
            </div>
          </div>
          {#if error !== ""}<p class="help is-danger">{error}</p>{/if}
          <div class="project-note-actions">
            <button
              class="button"
              type="button"
              onclick={() =>
                runtime.navigate(
                  recordsPath(workspace.id, currentRoute.projectID),
                )}>Cancel</button
            ><button class="button is-primary" type="submit" disabled={saving}
              >{saving ? "Saving..." : "Save record type"}</button
            >
          </div>
        </form>
        {:else if currentRoute.kind === "project-record"}<section>
          <PageHeading as="header">
            <p class="eyebrow">{schema?.label ?? "Record"}</p>
            <h2>
              {#if recordsStatus === "checking"}Loading record...{:else}{#each primaryLines(activeValues) as line}<span
                    class="is-block">{line}</span
                  >{/each}{/if}
            </h2>
          </PageHeading>
          <section class="card block mb-6">
            <div class="card-content">
              <h2 class="title is-4">Record</h2>
              {#if recordsStatus === "checking"}<p class="dashboard-empty">
                  Loading record...
                </p>{:else if activeValues.length === 0}<p
                  class="dashboard-empty"
                >
                  This record has no values.
                </p>{:else}<div>
                  {#each attributes as attribute (attribute.id)}{@const items =
                      activeValues.filter(
                        (item) => item.attribute === attribute.id,
                      )}{#if items.length !== 0}<div class="block">
                        <p class="heading">{attribute.label}</p>
                        <div>
                          {#each items as item (item.id)}{@const href =
                              referencePath(
                                attribute,
                                item,
                              )}{#if href}<RouterLink
                                class="is-block"
                                {href}
                                download={attribute.type === "file"
                                  ? item.file?.name ?? true
                                  : undefined}>{#each valueLines(attribute, item) as line}<span
                                    class="is-block"
                                    >{line}</span
                                  >{/each}</RouterLink
                              >{:else}<div>
                                {#each valueLines(attribute, item) as line}<span
                                    class="is-block"
                                    >{line}</span
                                  >{/each}
                              </div>{/if}{/each}
                        </div>
                      </div>{/if}{/each}
                </div>{/if}
              {#if recordsStatus !== "checking"}<div class="buttons mt-5">
                  <RouterLink
                    class="button is-primary is-small"
                    href={`${recordPath(workspace.id, currentRoute.projectID, currentRoute.schemaID, currentRoute.recordID)}/edit`}
                    >Edit record</RouterLink
                  >
                </div>
              {/if}
            </div>
          </section>
          {#if incomingReferences.length !== 0}<section class="card block">
              <div class="card-content">
                <h2 class="title is-4">Referenced by</h2>
                {#each incomingReferences as group (`${group.source_schema.id}-${group.source_attribute.id}`)}<div
                    class="block"
                  >
                    <p class="heading">
                      {group.source_schema.label} / {group.source_attribute
                        .label}
                    </p>
                    <div>
                      {#each group.references as reference, index (`${reference.id}-${index}`)}<RouterLink
                          class="is-block block"
                          href={recordPath(
                            workspace.id,
                            currentRoute.projectID,
                            group.source_schema.id,
                            reference.id,
                          )}
                          >{incomingReferenceLabel(group, reference)}</RouterLink
                        >{/each}
                    </div>
                  </div>{/each}
                {#if incomingReferencesNextCursor !== undefined}<button
                  class="button is-small"
                  type="button"
                  disabled={incomingReferencesLoading}
                  onclick={() => void loadMoreIncomingReferences()}
                  >{incomingReferencesLoading
                    ? "Loading..."
                    : "Show more"}</button
                  >{/if}
              </div>
            </section>{/if}
        </section>
      {:else if currentRoute.kind === "project-record-new" || currentRoute.kind === "project-record-edit"}<form
          class="record-editor record-edit-form"
          onsubmit={(event) => {
            event.preventDefault()
            void saveRecord()
          }}
        >
          {#snippet recordActions()}{#if currentRoute.kind === "project-record-edit"}<button
                class="button is-danger is-light"
                type="button"
                disabled={saving}
                onclick={() => void deleteRecord()}>Remove record</button
              >{/if}{/snippet}
          <PageHeading actions={recordActions}>
            <p class="eyebrow">{schema?.label ?? "Record"}</p>
            <h2>
              {currentRoute.kind === "project-record-new"
                ? "New record"
                : "Edit record"}
            </h2>
          </PageHeading>
          {#if recordsStatus === "checking"}<p class="dashboard-empty">
              Loading record...
            </p>{:else if attributes.length === 0}<p class="dashboard-empty">
              Add fields to this record type before creating records.
            </p>{:else}{#each attributes as attribute (attribute.id)}<fieldset
                class="record-value-field"
              >
                <legend
                  >{attribute.label}
                  <small
                    >{attribute.type}{attribute.cardinality === "many"
                      ? ", many"
                      : ""}</small
                  ></legend
                >{#if attribute.description !== ""}<p>
                    {attribute.description}
                  </p>{/if}{#each attributeValues(attribute.id) as draft (draft.id ?? `${draft.attribute}-${draftIndex(draft)}`)}{@const index =
                    draftIndex(draft)}
                  <div class="record-value-input">
                    {#if attribute.type === "boolean"}<label class="checkbox"
                        ><input
                          type="checkbox"
                          checked={draft.value === true}
                          onchange={(event) =>
                            setDraft(index, {
                              value: event.currentTarget.checked,
                            })}
                        /> Yes</label
                      >{:else if attribute.type === "file"}<div
                        class="select is-fullwidth"><select
                          required
                          value={valueLabel(draft.value)}
                          onchange={(event) =>
                            setDraft(index, {
                              value: event.currentTarget.value,
                            })}
                          ><option value="" disabled>Select project file</option
                          >{#each projectFiles as file (file.id)}<option
                              value={file.id}>{file.name}</option
                            >{/each}</select
                        ></div>
                      {:else}<input
                        class="input"
                        required
                        value={valueLabel(draft.value)}
                        type={attribute.type === "number" ? "number" : "text"}
                        placeholder={attribute.type === "datetime"
                          ? "RFC 3339 timestamp"
                          : attribute.type === "record"
                            ? "Record ID"
                            : ""}
                        oninput={(event) =>
                          setDraft(index, {
                            value: typedValue(
                              attribute,
                              event.currentTarget.value,
                            ),
                          })}
                      />{/if}<label class="checkbox sensitive-value"
                      ><input
                        type="checkbox"
                        checked={draft.sensitive}
                        onchange={(event) =>
                          setDraft(index, {
                            sensitive: event.currentTarget.checked,
                          })}
                      /> Sensitive</label
                    ><button
                      class="button is-small is-danger is-light"
                      type="button"
                      onclick={() =>
                        draft.id === undefined
                          ? (drafts = drafts.filter(
                              (candidate) => candidate !== draft,
                            ))
                          : setDraft(index, { removed: true })}>Remove</button
                    >
                  </div>{/each}{#if attribute.cardinality === "many"}<button
                    class="button is-small"
                    type="button"
                    onclick={() =>
                      (drafts = [...drafts, emptyDraft(attribute)])}
                    >Add value</button
                  >{/if}
              </fieldset>{/each}{/if}{#if error !== ""}<p
              class="help is-danger"
            >
              {error}
            </p>{/if}
          <div class="project-note-actions">
            <button
              class="button"
              type="button"
              onclick={() =>
                runtime.navigate(
                  currentRoute.kind === "project-record-edit"
                    ? recordPath(
                        workspace.id,
                        currentRoute.projectID,
                        currentRoute.schemaID,
                        currentRoute.recordID,
                      )
                    : schemaPath(
                        workspace.id,
                        currentRoute.projectID,
                        currentRoute.schemaID,
                      ),
                )}>Cancel</button
            ><button
              class="button is-primary"
              type="submit"
              disabled={saving || attributes.length === 0}
              >{saving ? "Saving..." : "Save record"}</button
            >
          </div>
        </form>
      {:else if currentRoute.kind === "project-record-schema"}{#snippet schemaListActions()}<div class="buttons">
              <RouterLink
                class="button is-small"
                href={`${schemaPath(workspace.id, currentRoute.projectID, currentRoute.schemaID)}/edit`}
                >Edit record type</RouterLink
              ><RouterLink
                class="button is-primary is-small"
                href={`${schemaPath(workspace.id, currentRoute.projectID, currentRoute.schemaID)}/new`}
                >New record</RouterLink
              >
            </div>{/snippet}
        <PageHeading actions={schemaListActions}>
          <p class="eyebrow">Record type</p>
          <h2>{schema?.label ?? "Records"}</h2>
          {#if schema?.description}<p class="subtitle is-6">
              {schema.description}
            </p>{/if}
        </PageHeading>
        {#if recordsStatus === "checking"}<p class="dashboard-empty">
            Loading records...
          </p>{:else if recordsStatus === "unavailable"}<p
            class="dashboard-empty"
          >
            Records could not be loaded.
          </p>{:else}<div class="columns is-multiline">
            {#each records as record (record.id)}<div
                class="column is-half-tablet is-one-third-desktop"
              >
                <RouterLink
                  class="card record-card"
                  href={recordPath(
                    workspace.id,
                    currentRoute.projectID,
                    currentRoute.schemaID,
                    record.id,
                  )}
                  ><div class="card-content p-4">
                    <p class="title is-5 is-block mb-4">
                      {#each primaryLines(record.values ?? []) as line}<span
                          class="is-block">{line}</span
                        >{/each}
                    </p>
                    {#each displayAttributes.filter((attribute) => attribute.display === "secondary") as attribute (attribute.id)}{@const item =
                        record.values?.find(
                          (candidate) => candidate.attribute === attribute.id,
                        )}{#if item !== undefined}<div class="block">
                          <p class="heading">{attribute.label}</p>
                          <div>
                            {#each valueLines(attribute, item) as line}<span
                                class="is-block">{line}</span
                              >{/each}
                          </div>
                        </div>{/if}{/each}
                  </div></RouterLink
                >
              </div>{:else}<p class="dashboard-empty">No records yet.</p>{/each}
          </div>{/if}
      {:else}{#snippet recordTypesActions()}<div>
            <RouterLink
              class="button is-primary"
              href={`${recordsPath(workspace.id, currentRoute.projectID)}/new`}
              >New record type</RouterLink
            >
          </div>{/snippet}
        <PageHeading actions={recordTypesActions}>
          <p class="eyebrow">Project records</p>
          <h2>Record Types</h2>
          <p class="subtitle is-6">
            Define reusable record types and their fields.
          </p>
        </PageHeading>
        {#if schemasStatus === "checking"}<p class="dashboard-empty">
            Loading record types...
          </p>{:else if schemasStatus === "unavailable"}<p
            class="dashboard-empty"
          >
            Record types could not be loaded.
          </p>{:else}<div class="columns is-multiline">
            {#each schemas as item (item.id)}<div
                class="column is-half-tablet is-one-third-desktop"
              >
                <RouterLink
                  class="card record-card"
                  href={schemaPath(
                    workspace.id,
                    currentRoute.projectID,
                    item.id,
                  )}
                  ><div class="card-content p-4">
                    <p class="title is-5">{item.label}</p>
                    {#if item.description !== ""}<p class="subtitle is-6">
                        {item.description}
                      </p>{/if}
                  </div></RouterLink
                >
              </div>{:else}<p class="dashboard-empty">
                No record types yet. Create one to begin tracking records.
              </p>{/each}
          </div>{/if}{/if}
      {#if currentRoute.kind === "project-record-schema-edit" && schema !== null}
        <section class="attribute-editor">
          <PageHeading>
            <p class="eyebrow">Record type fields</p>
            <h2>
              {attributeForm.id === undefined
                ? "Add field"
                : `Edit ${attributeForm.label}`}
            </h2>
          </PageHeading>
          <form
            onsubmit={(event) => {
              event.preventDefault()
              void saveAttribute()
            }}
          >
            <div class="record-field-grid">
              <div class="field">
                <label class="label" for="attribute-name">Name</label><input
                  class="input"
                  id="attribute-name"
                  required
                  disabled={attributeForm.id !== undefined}
                  bind:value={attributeForm.name}
                />
              </div>
              <div class="field">
                <label class="label" for="attribute-label">Label</label><input
                  class="input"
                  id="attribute-label"
                  required
                  bind:value={attributeForm.label}
                />
              </div>
              <div class="field">
                <label class="label" for="attribute-type">Type</label>
                <div class="select is-fullwidth">
                  <select id="attribute-type" bind:value={attributeForm.type}
                    ><option value="text">Text</option><option value="number"
                      >Number</option
                    ><option value="boolean">Boolean</option><option
                      value="datetime">Date and time</option
                      ><option value="record">Record reference</option><option
                        value="file">Project file reference</option></select
                  >
                </div>
              </div>
              {#if attributeForm.type === "record"}<div class="field">
                  <label class="label" for="attribute-target"
                    >Target record type</label
                  >
                  <div class="select is-fullwidth">
                    <select
                      id="attribute-target"
                      required
                      bind:value={attributeForm.target_schema}
                      ><option value="" disabled>Select record type</option
                      >{#each schemas as item (item.id)}<option value={item.id}
                          >{item.label}</option
                        >{/each}</select
                    >
                  </div>
                </div>{/if}
              <div class="field">
                <label class="label" for="attribute-cardinality">Values</label>
                <div class="select is-fullwidth">
                  <select
                    id="attribute-cardinality"
                    bind:value={attributeForm.cardinality}
                    ><option value="one">One</option><option value="many"
                      >Many</option
                    ></select
                  >
                </div>
              </div>
              <div class="field">
                <label class="label" for="attribute-uniqueness"
                  >Uniqueness</label
                >
                <div class="select is-fullwidth">
                  <select
                    id="attribute-uniqueness"
                    bind:value={attributeForm.uniqueness}
                    ><option value="none">None</option><option value="record"
                      >Within record</option
                    ><option value="global">Across records</option></select
                  >
                </div>
              </div>
              <div class="field">
                <label class="label" for="attribute-display">Card display</label
                >
                <div class="select is-fullwidth">
                  <select
                    id="attribute-display"
                    bind:value={attributeForm.display}
                    ><option value="none">Hidden</option><option value="primary"
                      >Primary</option
                    ><option value="secondary">Secondary</option></select
                  >
                </div>
              </div>
              <div class="field">
                <label class="label" for="attribute-display-order"
                  >Display order</label
                ><input
                  class="input"
                  id="attribute-display-order"
                  type="number"
                  min="0"
                  step="1"
                  required
                  bind:value={attributeForm.display_order}
                />
                <p class="help">
                  Lower numbers appear first; name breaks ties.
                </p>
              </div>
            </div>
            <div class="field">
              <label class="label" for="attribute-description"
                >Description</label
              ><textarea
                class="textarea"
                id="attribute-description"
                rows="2"
                bind:value={attributeForm.description}></textarea>
            </div>
            <div class="project-note-actions">
              <button class="button" type="button" onclick={resetAttributeForm}
                >Cancel</button
              ><button class="button is-primary" type="submit" disabled={saving}
                >{saving ? "Saving..." : "Save field"}</button
              >
            </div>
          </form>
          <div class="collection-list">
            {#each attributes as attribute (attribute.id)}<div
                class="dashboard-row record-attribute-row"
              >
                <span class="dashboard-row-content"
                  ><strong>{attribute.label}</strong><span
                    >{attribute.name} · {attribute.type} · {attribute.cardinality}</span
                  ></span
                ><span class="project-note-actions"
                  ><button
                    class="button is-small"
                    type="button"
                    onclick={() => editAttribute(attribute)}>Edit</button
                  ><button
                    class="button is-small is-danger is-light"
                    type="button"
                    onclick={() => void deleteAttribute(attribute)}
                    >Remove</button
                  ></span
                >
              </div>{:else}<p class="dashboard-empty">No fields yet.</p>{/each}
          </div>
        </section>
      {/if}
      </PageBody></SidebarPage.Body>
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
