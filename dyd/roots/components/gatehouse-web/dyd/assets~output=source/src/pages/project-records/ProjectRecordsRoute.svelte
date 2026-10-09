<script lang="ts">
  import { PageBody, PageHeading, RouterLink, StatusPage } from "../../ui"
  import { workspaceNavigation } from "../../app/navigation"
  import * as UI from "../../ui"
  import { fetchProject, type Project } from "../../app/projects"

  import { fetchProjectFiles, type ProjectFile } from "../../app/project-files"
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
  import * as SidebarPage from "../../ui/sidebar-page"

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
  const projectFilePath = (
    workspaceID: string,
    projectID: string,
    fileID: string,
  ) =>
    `${projectPath(workspaceID, projectID)}/files/${encodeURIComponent(fileID)}`
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
      const [schemaResponse, attributesResponse, filesResponse] =
        await Promise.all([
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
    if (workspace === null) return undefined
    if (attribute.type === "file" && typeof value.value === "string")
      return projectFilePath(workspace.id, currentRoute.projectID, value.value)
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
      runtime.navigate(
        schemaPath(route.workspaceID, route.projectID, route.schemaID),
      )
    } catch {
      error = "The record could not be removed."
    } finally {
      saving = false
    }
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <StatusPage busy>
    {#snippet children()}
      <UI.Text as="p">Loading your workspaces.</UI.Text>
    {/snippet}
  </StatusPage>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage title="Connection unavailable">
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => void runtime.refresh()}>Try again</UI.Button
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else if auth.state.status !== "authenticated"}
  <StatusPage title="Sign in required">
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => runtime.requireLogin()}>Sign in</UI.Button
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <StatusPage title="No workspace access" />
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar
      ><UI.Navigation
        {...workspaceNavigation(runtime, workspace, "projects")}
      /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header>
        <SidebarPage.Toggle />
        <UI.Breadcrumb.Root aria-label="Breadcrumb">
          <UI.Breadcrumb.List>
            <UI.Breadcrumb.Item
              ><RouterLink href={`/app/wsp/${encodeURIComponent(workspace.id)}`}
                >{workspace.name ?? workspace.id}</RouterLink
              ></UI.Breadcrumb.Item
            >
            <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
              ><RouterLink
                href={`/app/wsp/${encodeURIComponent(workspace.id)}/prj`}
                >Projects</RouterLink
              ></UI.Breadcrumb.Item
            >
            <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
              ><RouterLink
                href={projectPath(workspace.id, currentRoute.projectID)}
                >{project?.name ?? "Project"}</RouterLink
              ></UI.Breadcrumb.Item
            >
            <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
              ><UI.Breadcrumb.Page>Records</UI.Breadcrumb.Page
              ></UI.Breadcrumb.Item
            >
          </UI.Breadcrumb.List>
        </UI.Breadcrumb.Root>
      </SidebarPage.Header>
      <SidebarPage.Body
        ><PageBody fluid>
          {#if projectStatus === "checking"}<UI.Text as="p" tone="muted">
              Loading project...
            </UI.Text>
          {:else if projectStatus === "unavailable"}<UI.Text
              as="p"
              tone="muted"
            >
              Project unavailable.
            </UI.Text>
            <UI.Button
              variant="default"
              type="button"
              onclick={() =>
                void loadRoute(
                  currentRoute,
                  generation,
                  abortController!.signal,
                )}>Try again</UI.Button
            >
          {:else if currentRoute.kind === "project-record-schema-new" || currentRoute.kind === "project-record-schema-edit"}{#snippet schemaActions()}{#if currentRoute.kind === "project-record-schema-edit"}<UI.Button
                  variant="outline"
                  type="button"
                  disabled={saving}
                  onclick={() => void deleteSchema()}
                  >Remove record type</UI.Button
                >{/if}{/snippet}
            <UI.Form
              onsubmit={(event) => {
                event.preventDefault()
                void saveSchema()
              }}
            >
              <PageHeading actions={schemaActions}>
                <UI.Text as="p" size="eyebrow">Record type</UI.Text>
                <UI.Heading level={2}>
                  {currentRoute.kind === "project-record-schema-new"
                    ? "New record type"
                    : `Edit ${schema?.label ?? "record type"}`}
                </UI.Heading>
              </PageHeading>
              <UI.Field>
                <UI.Label for="schema-name">Name</UI.Label>
                <UI.Input
                  id="schema-name"
                  required
                  disabled={currentRoute.kind !== "project-record-schema-new"}
                  maxlength={64}
                  bind:value={schemaName}
                  placeholder="contacts"
                />
                <UI.Text as="p" size="small">
                  Lowercase letters, numbers, and underscores only.
                </UI.Text>
              </UI.Field>
              <UI.Field>
                <UI.Label for="schema-label">Label</UI.Label>
                <UI.Input
                  id="schema-label"
                  required
                  maxlength={256}
                  bind:value={schemaLabel}
                />
              </UI.Field>
              <UI.Field>
                <UI.Label for="schema-description">Description</UI.Label>
                <UI.Textarea
                  id="schema-description"
                  rows={3}
                  maxlength={4096}
                  bind:value={schemaDescription}
                ></UI.Textarea>
              </UI.Field>
              {#if error !== ""}<UI.Text as="p" size="small" role="alert"
                  >{error}</UI.Text
                >{/if}
              <UI.Row>
                <UI.Button
                  variant="outline"
                  type="button"
                  onclick={() =>
                    runtime.navigate(
                      recordsPath(workspace.id, currentRoute.projectID),
                    )}>Cancel</UI.Button
                ><UI.Button
                  variant="outline"
                  size="sm"
                  type="submit"
                  disabled={saving}
                  >{saving ? "Saving..." : "Save record type"}</UI.Button
                >
              </UI.Row>
            </UI.Form>
          {:else if currentRoute.kind === "project-record"}{#snippet recordActions()}<UI.Button
                variant="default"
                type="button"
                onclick={() =>
                  runtime.navigate(
                    `${recordPath(workspace.id, currentRoute.projectID, currentRoute.schemaID, currentRoute.recordID)}/edit`,
                  )}>Edit record</UI.Button
              >{/snippet}
            <UI.Stack as="section">
              <PageHeading as="header" actions={recordActions}>
                <UI.Text as="p" size="eyebrow"
                  >{schema?.label ?? "Record"}</UI.Text
                >
                <UI.Heading level={2}>
                  {#if recordsStatus === "checking"}Loading record...{:else}<UI.Stack
                      as="span"
                      >{#each primaryLines(activeValues) as line}<UI.Text
                          >{line}</UI.Text
                        >{/each}</UI.Stack
                    >{/if}
                </UI.Heading>
              </PageHeading>
              <UI.Stack as="section" aria-labelledby="record-heading">
                <UI.Heading level={3} id="record-heading">Record</UI.Heading>
                {#if recordsStatus === "checking"}<UI.Text as="p" tone="muted">
                    Loading record...
                  </UI.Text>{:else if activeValues.length === 0}<UI.Text
                    as="p"
                    tone="muted"
                  >
                    This record has no values.
                  </UI.Text>{:else}{#each attributes as attribute (attribute.id)}{@const items =
                      activeValues.filter(
                        (item) => item.attribute === attribute.id,
                      )}{#if items.length !== 0}<UI.Stack
                        as="section"
                        gap="tight"
                      >
                        <UI.Text as="p" size="eyebrow"
                          >{attribute.label}</UI.Text
                        >
                        <UI.List>
                          {#each items as item (item.id)}{@const href =
                              referencePath(
                                attribute,
                                item,
                              )}{#if href}<RouterLink {href}
                                >{valueLines(attribute, item).join(
                                  " / ",
                                )}</RouterLink
                              >{:else}<UI.Group>
                                <UI.Stack as="span"
                                  >{#each valueLines(attribute, item) as line}<UI.Text
                                      >{line}</UI.Text
                                    >{/each}</UI.Stack
                                >
                              </UI.Group>{/if}{/each}
                        </UI.List>
                      </UI.Stack>{/if}{/each}{/if}
              </UI.Stack>
              {#if incomingReferences.length !== 0}<UI.Stack
                  as="section"
                  aria-labelledby="referenced-by-heading"
                >
                  <UI.Heading level={3} id="referenced-by-heading"
                    >Referenced by</UI.Heading
                  >
                  {#each incomingReferences as group (`${group.source_schema.id}-${group.source_attribute.id}`)}<UI.Stack
                      as="section"
                      gap="tight"
                    >
                      <UI.Text as="p" size="eyebrow">
                        {group.source_schema.label} / {group.source_attribute
                          .label}
                      </UI.Text>
                      <UI.List>
                        {#each group.references as reference, index (`${reference.id}-${index}`)}<RouterLink
                            href={recordPath(
                              workspace.id,
                              currentRoute.projectID,
                              group.source_schema.id,
                              reference.id,
                            )}
                            >{incomingReferenceLabel(
                              group,
                              reference,
                            )}</RouterLink
                          >{/each}
                      </UI.List>
                    </UI.Stack>{/each}
                  {#if incomingReferencesNextCursor !== undefined}<UI.Button
                      variant="outline"
                      size="sm"
                      type="button"
                      disabled={incomingReferencesLoading}
                      onclick={() => void loadMoreIncomingReferences()}
                      >{incomingReferencesLoading
                        ? "Loading..."
                        : "Show more"}</UI.Button
                    >{/if}
                </UI.Stack>{/if}
            </UI.Stack>
          {:else if currentRoute.kind === "project-record-new" || currentRoute.kind === "project-record-edit"}{#snippet recordActions()}{#if currentRoute.kind === "project-record-edit"}<UI.Button
                  variant="outline"
                  type="button"
                  disabled={saving}
                  onclick={() => void deleteRecord()}>Remove record</UI.Button
                >{/if}{/snippet}
            <UI.Form
              onsubmit={(event) => {
                event.preventDefault()
                void saveRecord()
              }}
            >
              <PageHeading actions={recordActions}>
                <UI.Text as="p" size="eyebrow"
                  >{schema?.label ?? "Record"}</UI.Text
                >
                <UI.Heading level={2}>
                  {currentRoute.kind === "project-record-new"
                    ? "New record"
                    : "Edit record"}
                </UI.Heading>
              </PageHeading>
              {#if recordsStatus === "checking"}<UI.Text as="p" tone="muted">
                  Loading record...
                </UI.Text>{:else if attributes.length === 0}<UI.Text
                  as="p"
                  tone="muted"
                >
                  Add fields to this record type before creating records.
                </UI.Text>{:else}{#each attributes as attribute (attribute.id)}<UI.FieldSet
                  >
                    <UI.FieldLegend>{attribute.label}</UI.FieldLegend>
                    {#if attribute.description !== ""}<UI.Text as="p">
                        {attribute.description}
                      </UI.Text>{/if}{#each attributeValues(attribute.id) as draft (draft.id ?? `${draft.attribute}-${draftIndex(draft)}`)}{@const index =
                        draftIndex(draft)}
                      <UI.Stack gap="tight">
                        <UI.Row justify="between">
                          <UI.Field grow>
                            {#if attribute.type === "boolean"}<UI.Label
                                layout="choice"
                                ><UI.Checkbox
                                  checked={draft.value === true}
                                  onCheckedChange={(checked) =>
                                    setDraft(index, {
                                      value: checked,
                                    })}
                                /> Yes</UI.Label
                              >{:else if attribute.type === "file"}<UI.Select
                                required
                                value={valueLabel(draft.value)}
                                onchange={(event) =>
                                  setDraft(index, {
                                    value: event.currentTarget.value,
                                  })}
                                ><UI.Option value="" disabled
                                  >Select project file</UI.Option
                                >{#each projectFiles as file (file.id)}<UI.Option
                                    value={file.id}>{file.name}</UI.Option
                                  >{/each}</UI.Select
                              >
                            {:else}<UI.Input
                                required
                                value={valueLabel(draft.value)}
                                type={attribute.type === "number"
                                  ? "number"
                                  : "text"}
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
                              />{/if}
                          </UI.Field>
                          <UI.Button
                            variant="outline"
                            type="button"
                            onclick={() =>
                              draft.id === undefined
                                ? (drafts = drafts.filter(
                                    (candidate) => candidate !== draft,
                                  ))
                                : setDraft(index, { removed: true })}
                            >Remove</UI.Button
                          >
                        </UI.Row>
                        <UI.Label layout="choice"
                          ><UI.Checkbox
                            checked={draft.sensitive}
                            onCheckedChange={(checked) =>
                              setDraft(index, {
                                sensitive: checked,
                              })}
                          /> Sensitive</UI.Label
                        >
                      </UI.Stack>{/each}{#if attribute.cardinality === "many"}<UI.Button
                        variant="outline"
                        size="sm"
                        type="button"
                        onclick={() =>
                          (drafts = [...drafts, emptyDraft(attribute)])}
                        >Add value</UI.Button
                      >{/if}
                  </UI.FieldSet>{/each}{/if}{#if error !== ""}<UI.Text
                  as="p"
                  size="small"
                  role="alert"
                >
                  {error}
                </UI.Text>{/if}
              <UI.Row>
                <UI.Button
                  variant="outline"
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
                    )}>Cancel</UI.Button
                ><UI.Button
                  variant="default"
                  type="submit"
                  disabled={saving || attributes.length === 0}
                  >{saving ? "Saving..." : "Save record"}</UI.Button
                >
              </UI.Row>
            </UI.Form>
          {:else if currentRoute.kind === "project-record-schema"}{#snippet schemaListActions()}<UI.Button
                variant="outline"
                size="sm"
                type="button"
                onclick={() =>
                  runtime.navigate(
                    `${schemaPath(workspace.id, currentRoute.projectID, currentRoute.schemaID)}/edit`,
                  )}>Edit record type</UI.Button
              ><UI.Button
                variant="default"
                size="sm"
                type="button"
                onclick={() =>
                  runtime.navigate(
                    `${schemaPath(workspace.id, currentRoute.projectID, currentRoute.schemaID)}/new`,
                  )}>New record</UI.Button
              >{/snippet}
            <PageHeading as="header" actions={schemaListActions}>
              <UI.Text as="p" size="eyebrow">Record type</UI.Text>
              <UI.Heading level={2}>{schema?.label ?? "Records"}</UI.Heading>
              {#if schema?.description}<UI.Text as="p" tone="muted">
                  {schema.description}
                </UI.Text>{/if}
            </PageHeading>
            {#if recordsStatus === "checking"}<UI.Text as="p" tone="muted">
                Loading records...
              </UI.Text>{:else if recordsStatus === "unavailable"}<UI.Text
                as="p"
                tone="muted"
              >
                Records could not be loaded.
              </UI.Text>{:else}<UI.Grid>
                {#each records as record (record.id)}<RouterLink
                    variant="card"
                    href={recordPath(
                      workspace.id,
                      currentRoute.projectID,
                      currentRoute.schemaID,
                      record.id,
                    )}
                    ><UI.Heading level={4}>
                      {#each primaryLines(record.values ?? []) as line}<UI.Text
                          >{line}</UI.Text
                        >{/each}
                    </UI.Heading>
                    {#each displayAttributes.filter((attribute) => attribute.display === "secondary") as attribute (attribute.id)}{@const item =
                        record.values?.find(
                          (candidate) => candidate.attribute === attribute.id,
                        )}{#if item !== undefined}<UI.Stack
                          as="section"
                          gap="tight"
                        >
                          <UI.Text size="eyebrow">{attribute.label}</UI.Text>
                          <UI.Stack as="span" gap="tight">
                            {#each valueLines(attribute, item) as line}<UI.Text
                                >{line}</UI.Text
                              >{/each}
                          </UI.Stack>
                        </UI.Stack>{/if}{/each}</RouterLink
                  >{:else}<UI.Text as="p" tone="muted">
                    No records yet.
                  </UI.Text>{/each}
              </UI.Grid>{/if}
          {:else}{#snippet recordTypesActions()}<UI.Button
                variant="default"
                type="button"
                onclick={() =>
                  runtime.navigate(
                    `${recordsPath(workspace.id, currentRoute.projectID)}/new`,
                  )}>New record type</UI.Button
              >{/snippet}
            <PageHeading as="header" actions={recordTypesActions}>
              <UI.Text as="p" size="eyebrow">Project records</UI.Text>
              <UI.Heading level={2}>Record Types</UI.Heading>
              <UI.Text as="p" tone="muted">
                Define reusable record types and their fields.
              </UI.Text>
            </PageHeading>
            {#if schemasStatus === "checking"}<UI.Text as="p" tone="muted">
                Loading record types...
              </UI.Text>{:else if schemasStatus === "unavailable"}<UI.Text
                as="p"
                tone="muted"
              >
                Record types could not be loaded.
              </UI.Text>{:else}<UI.Grid>
                {#each schemas as item (item.id)}<RouterLink
                    variant="card"
                    href={schemaPath(
                      workspace.id,
                      currentRoute.projectID,
                      item.id,
                    )}
                    ><UI.Heading level={4}>{item.label}</UI.Heading>
                    {#if item.description !== ""}<UI.Text size="small">
                        {item.description}
                      </UI.Text>{/if}</RouterLink
                  >{:else}<UI.Text as="p" tone="muted">
                    No record types yet. Create one to begin tracking records.
                  </UI.Text>{/each}
              </UI.Grid>{/if}{/if}
          {#if currentRoute.kind === "project-record-schema-edit" && schema !== null}
            <UI.Stack as="section">
              <PageHeading>
                <UI.Text as="p" size="eyebrow">Record type fields</UI.Text>
                <UI.Heading level={2}>
                  {attributeForm.id === undefined
                    ? "Add field"
                    : `Edit ${attributeForm.label}`}
                </UI.Heading>
              </PageHeading>
              <UI.Form
                onsubmit={(event) => {
                  event.preventDefault()
                  void saveAttribute()
                }}
              >
                <UI.Grid>
                  <UI.Field>
                    <UI.Label for="attribute-name">Name</UI.Label><UI.Input
                      id="attribute-name"
                      required
                      disabled={attributeForm.id !== undefined}
                      bind:value={attributeForm.name}
                    />
                  </UI.Field>
                  <UI.Field>
                    <UI.Label for="attribute-label">Label</UI.Label><UI.Input
                      id="attribute-label"
                      required
                      bind:value={attributeForm.label}
                    />
                  </UI.Field>
                  <UI.Field>
                    <UI.Label for="attribute-type">Type</UI.Label>
                    <UI.Select
                      id="attribute-type"
                      bind:value={attributeForm.type}
                      ><UI.Option value="text">Text</UI.Option><UI.Option
                        value="number">Number</UI.Option
                      ><UI.Option value="boolean">Boolean</UI.Option><UI.Option
                        value="datetime">Date and time</UI.Option
                      ><UI.Option value="record">Record reference</UI.Option
                      ><UI.Option value="file">Project file reference</UI.Option
                      ></UI.Select
                    >
                  </UI.Field>
                  {#if attributeForm.type === "record"}<UI.Field>
                      <UI.Label for="attribute-target"
                        >Target record type</UI.Label
                      >
                      <UI.Select
                        id="attribute-target"
                        required
                        bind:value={attributeForm.target_schema}
                        ><UI.Option value="" disabled
                          >Select record type</UI.Option
                        >{#each schemas as item (item.id)}<UI.Option
                            value={item.id}>{item.label}</UI.Option
                          >{/each}</UI.Select
                      >
                    </UI.Field>{/if}
                  <UI.Field>
                    <UI.Label for="attribute-cardinality">Values</UI.Label>
                    <UI.Select
                      id="attribute-cardinality"
                      bind:value={attributeForm.cardinality}
                      ><UI.Option value="one">One</UI.Option><UI.Option
                        value="many">Many</UI.Option
                      ></UI.Select
                    >
                  </UI.Field>
                  <UI.Field>
                    <UI.Label for="attribute-uniqueness">Uniqueness</UI.Label>
                    <UI.Select
                      id="attribute-uniqueness"
                      bind:value={attributeForm.uniqueness}
                      ><UI.Option value="none">None</UI.Option><UI.Option
                        value="record">Within record</UI.Option
                      ><UI.Option value="global">Across records</UI.Option
                      ></UI.Select
                    >
                  </UI.Field>
                  <UI.Field>
                    <UI.Label for="attribute-display">Card display</UI.Label>
                    <UI.Select
                      id="attribute-display"
                      bind:value={attributeForm.display}
                      ><UI.Option value="none">Hidden</UI.Option><UI.Option
                        value="primary">Primary</UI.Option
                      ><UI.Option value="secondary">Secondary</UI.Option
                      ></UI.Select
                    >
                  </UI.Field>
                  <UI.Field>
                    <UI.Label for="attribute-display-order"
                      >Display order</UI.Label
                    ><UI.Input
                      id="attribute-display-order"
                      type="number"
                      min="0"
                      step="1"
                      required
                      bind:value={attributeForm.display_order}
                    />
                    <UI.Text as="p" size="small">
                      Lower numbers appear first; name breaks ties.
                    </UI.Text>
                  </UI.Field>
                </UI.Grid>
                <UI.Field>
                  <UI.Label for="attribute-description">Description</UI.Label
                  ><UI.Textarea
                    id="attribute-description"
                    rows={2}
                    bind:value={attributeForm.description}
                  ></UI.Textarea>
                </UI.Field>
                <UI.Row>
                  <UI.Button
                    variant="outline"
                    type="button"
                    onclick={resetAttributeForm}>Cancel</UI.Button
                  ><UI.Button variant="default" type="submit" disabled={saving}
                    >{saving ? "Saving..." : "Save field"}</UI.Button
                  >
                </UI.Row>
              </UI.Form>
              <UI.List>
                {#each attributes as attribute (attribute.id)}<UI.Item>
                    <UI.Stack as="span"
                      ><UI.Text as="strong" emphasis="strong"
                        >{attribute.label}</UI.Text
                      ><UI.Text
                        >{attribute.name} · {attribute.type} · {attribute.cardinality}</UI.Text
                      ></UI.Stack
                    ><UI.Row as="span"
                      ><UI.Button
                        variant="outline"
                        size="sm"
                        type="button"
                        onclick={() => editAttribute(attribute)}>Edit</UI.Button
                      ><UI.Button
                        variant="outline"
                        size="sm"
                        type="button"
                        onclick={() => void deleteAttribute(attribute)}
                        >Remove</UI.Button
                      ></UI.Row
                    >
                  </UI.Item>{:else}<UI.Text as="p" tone="muted">
                    No fields yet.
                  </UI.Text>{/each}
              </UI.List>
            </UI.Stack>
          {/if}
        </PageBody></SidebarPage.Body
      >
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
