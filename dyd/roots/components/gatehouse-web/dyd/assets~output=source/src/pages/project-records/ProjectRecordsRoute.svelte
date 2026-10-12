<script lang="ts">
  import { untrack } from "svelte"
  import { createLoadState } from "../../utils/load-state.svelte"
  import { PageBody, PageHeading, RouterLink, StatusPage } from "../../ui"
  import { workspaceNavigation } from "../../app/navigation"
  import * as UI from "../../ui"
  import { setRecordReference } from "../../app/record-reference-drafts"
  import { createCollectionPage } from "../../utils/collection-page.svelte"
  import { dateTimeInput, dateTimeValue } from "../../utils/date-time-input"
  import ProjectFilePicker from "./ProjectFilePicker.svelte"
  import ProjectRecordPicker from "./ProjectRecordPicker.svelte"
  import ProjectRecordTypePicker from "./ProjectRecordTypePicker.svelte"
  import { fetchProject, type Project } from "../../app/projects"

  import {
    createProjectRecord,
    createProjectRecordAttribute,
    createProjectRecordSchema,
    fetchProjectRecord,
    fetchProjectRecordAttributes,
    fetchProjectRecordAttribute,
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
        | "project-record-schema-overview"
        | "project-record-fields"
        | "project-record-field-new"
        | "project-record-field-edit"
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
    changed?: boolean
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
  const projectLoad = createLoadState()
  const schemasLoad = createLoadState()
  const schemaLoad = createLoadState()
  const recordsLoad = createLoadState()
  const recordLoad = createLoadState()
  const referencesLoad = createLoadState()
  const typesPage = createCollectionPage()
  const recordsPage = createCollectionPage()
  const valuesPage = createCollectionPage()
  const fieldsPage = createCollectionPage()
  const incomingPage = createCollectionPage()
  const reloadFields = () =>
    isSchemaRoute(currentRoute)
      ? loadSchema(currentRoute, generation, abortController!.signal)
      : Promise.resolve(false)
  const reloadTypes = () =>
    loadSchemas(currentRoute, generation, abortController!.signal)
  const reloadRecords = () =>
    currentRoute.kind === "project-record-schema"
      ? loadRecords(currentRoute, generation, abortController!.signal)
      : Promise.resolve(false)
  const reloadValues = () =>
    "recordID" in currentRoute
      ? loadRecord(currentRoute, generation, abortController!.signal, false)
      : Promise.resolve(false)
  const { activity, access, auth } = runtime
  let project = $state<Project | null>(null)
  const projectStatus = $derived(projectLoad.status)
  let schemas = $state<ProjectRecordSchema[]>([])
  const schemasStatus = $derived(schemasLoad.status)
  let schema = $state<ProjectRecordSchema | null>(null)
  let attributes = $state<ProjectRecordAttribute[]>([])
  let records = $state<ProjectRecord[]>([])
  let active = $state<ProjectRecord | null>(null)
  let activeValues = $state<ProjectRecordValue[]>([])
  let incomingReferences = $state<ProjectRecordIncomingReferenceGroup[]>([])
  let incomingReferencesNextCursor = $state<string | undefined>(undefined)
  const incomingReferencesLoading = $derived(referencesLoad.state.pending)
  let drafts = $state<ValueDraft[]>([])
  let allValuesLoaded = $state(false)
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
  let draftSeeded = false
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as RecordsRoute)
  const recordsStatus = $derived(
    "recordID" in currentRoute
      ? recordLoad.status
      : currentRoute.kind === "project-record-schema"
        ? recordsLoad.status
        : "ready",
  )
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

  const activation = $derived(
    JSON.stringify([
      currentRoute,
      auth.state.status,
      auth.state.claims?.principal.ref.id,
      access.state.workspaceStatus,
      workspace !== null,
    ]),
  )
  $effect(() => {
    activation
    return untrack(() => activate(currentRoute))
  })

  function activate(route: RecordsRoute): () => void {
    typesPage.reset()
    recordsPage.reset()
    valuesPage.reset()
    fieldsPage.reset()
    incomingPage.reset()
    projectLoad.reset()
    schemasLoad.reset()
    schemaLoad.reset()
    recordsLoad.reset()
    recordLoad.reset()
    referencesLoad.reset()
    draftSeeded = false
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    project = null
    schemas = []
    schema = null
    attributes = []
    records = []
    active = null
    activeValues = []
    incomingReferences = []
    incomingReferencesNextCursor = undefined
    drafts = []
    allValuesLoaded = false
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
        projectLoad.stop()
        schemasLoad.stop()
        schemaLoad.stop()
        recordsLoad.stop()
        recordLoad.stop()
        referencesLoad.stop()
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
    const request = projectLoad.begin(signal)
    signal = request.signal
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
      const loaded = (await response.json()) as Project
      if (
        !request.commit(() => {
          project = loaded
        })
      )
        return
      subscribe(route, value)
      if (route.kind === "project-records") {
        await loadSchemas(route, value, signal)
        return
      }
      if (route.kind === "project-record-schema-new") return
      if (!isSchemaRoute(route)) return
      if (!(await loadSchema(route, value, signal))) return
      if (!isCurrent(value, route) || signal.aborted) return
      if (
        route.kind === "project-record-fields" ||
        route.kind === "project-record-field-new" ||
        route.kind === "project-record-schema-overview"
      )
        return
      if (route.kind === "project-record-field-edit") {
        const field = attributes.find((item) => item.id === route.attributeID)
        if (field) {
          if (!draftSeeded) {
            editAttribute(field)
            draftSeeded = true
          }
        } else
          runtime.navigate(
            `${schemaPath(route.workspaceID, route.projectID, route.schemaID)}/fields`,
            true,
          )
        return
      }
      if (route.kind === "project-record-schema-edit") {
        if (!draftSeeded) {
          schemaLabel = schema?.label ?? ""
          schemaDescription = schema?.description ?? ""
          draftSeeded = true
        }
        return
      }
      if (route.kind === "project-record-new") {
        if (!draftSeeded) {
          drafts = []
          draftSeeded = true
        }
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
        request.fail(new Error("Project could not be loaded."))
    } finally {
      request.finish()
    }
  }

  function subscribe(route: RecordsRoute, value: number): void {
    unsubscribe?.()
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
        if (route.kind.endsWith("-edit") || route.kind.endsWith("-new")) return
        if (
          route.kind === "project-records" &&
          !(await loadSchemas(route, value, signal))
        )
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
    const request = schemasLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
    try {
      const response = await fetchProjectRecordSchemas(
        route.workspaceID,
        route.projectID,
        signal,
        typesPage.params(),
      )
      if (!isCurrent(value, route) || signal.aborted) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error()
      const loaded = (await response.json()) as ProjectRecordSchema[]
      return request.commit(() => {
        schemas = loaded
        typesPage.accept(response)
      })
    } catch {
      if (isCurrent(value, route) && !signal.aborted)
        request.fail(new Error("Record types could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  async function loadSchema(
    route: Extract<RecordsRoute, { schemaID: string }>,
    value: number,
    signal: AbortSignal,
  ): Promise<boolean> {
    const request = schemaLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
    try {
      const [schemaResponse, attributesResponse] = await Promise.all([
        fetchProjectRecordSchema(
          route.workspaceID,
          route.projectID,
          route.schemaID,
          signal,
        ),
        route.kind === "project-record-field-edit"
          ? fetchProjectRecordAttribute(
              route.workspaceID,
              route.projectID,
              route.schemaID,
              route.attributeID,
              signal,
            )
          : route.kind === "project-record-field-new" ||
              route.kind === "project-record-schema-overview" ||
              route.kind === "project-record-schema-edit"
            ? Promise.resolve(null)
            : fetchProjectRecordAttributes(
                route.workspaceID,
                route.projectID,
                route.schemaID,
                signal,
                route.kind === "project-record-fields"
                  ? fieldsPage.params()
                  : undefined,
              ),
      ])
      if (!isCurrent(value, route) || signal.aborted) return false
      if (schemaResponse.status === 401 || attributesResponse?.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (schemaResponse.status === 404) {
        runtime.navigate(recordsPath(route.workspaceID, route.projectID), true)
        return false
      }
      if (!schemaResponse.ok || (attributesResponse && !attributesResponse.ok))
        throw new Error()
      const loaded = (await schemaResponse.json()) as ProjectRecordSchema
      let fields: ProjectRecordAttribute[] | undefined
      if (attributesResponse) {
        const result = (await attributesResponse.json()) as
          | ProjectRecordAttribute
          | ProjectRecordAttribute[]
        if (!isCurrent(value, route) || signal.aborted) return false
        fields = Array.isArray(result) ? result : [result]
      }
      return request.commit(() => {
        schema = loaded
        schemaName = loaded.name
        if (fields) attributes = fields
        if (route.kind === "project-record-fields" && attributesResponse)
          fieldsPage.accept(attributesResponse)
      })
    } catch {
      request.fail(new Error("The record type could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  async function loadRecords(
    route: Extract<RecordsRoute, { kind: "project-record-schema" }>,
    value: number,
    signal: AbortSignal,
  ): Promise<boolean> {
    const request = recordsLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
    try {
      const response = await fetchProjectRecords(
        route.workspaceID,
        route.projectID,
        route.schemaID,
        signal,
        recordsPage.params(),
      )
      if (!isCurrent(value, route) || signal.aborted) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error()
      const body = (await response.json()) as {
        records: ProjectRecord[]
        next_cursor?: string
      }
      if (!isCurrent(value, route) || signal.aborted) return false
      return request.commit(() => {
        records = body.records
        recordsPage.state.next = body.next_cursor ?? ""
      })
    } catch {
      if (isCurrent(value, route) && !signal.aborted)
        request.fail(new Error("Records could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  async function loadRecord(
    route: Extract<RecordsRoute, { recordID: string }>,
    value: number,
    signal: AbortSignal,
    refreshTitle = true,
  ): Promise<boolean> {
    const request = recordLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
    const requestedValues = valuesPage.key()
    try {
      const [recordResponse, valuesResponse] = await Promise.all([
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
          valuesPage.params(),
        ),
      ])
      if (!isCurrent(value, route) || signal.aborted) return false
      if (recordResponse.status === 401 || valuesResponse.status === 401) {
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
      if (!recordResponse.ok || !valuesResponse.ok) throw new Error()
      const loaded = (await recordResponse.json()) as ProjectRecord
      if (!request.current()) return false
      if (!active || refreshTitle) {
        const summaryResponse = await fetchProjectRecords(
          route.workspaceID,
          route.projectID,
          route.schemaID,
          signal,
          { name: route.recordID, limit: 1 },
        )
        if (!request.current()) return false
        if (summaryResponse.status === 401) {
          runtime.requireLogin()
          return false
        }
        if (!summaryResponse.ok)
          throw new Error("Record title could not be loaded.")
        loaded.values = (
          (await summaryResponse.json()) as { records: ProjectRecord[] }
        ).records[0]?.values
      } else loaded.values = active.values
      if (
        !isCurrent(value, route) ||
        signal.aborted ||
        requestedValues !== valuesPage.key()
      )
        return false
      const values = (await valuesResponse.json()) as {
        values: ProjectRecordValue[]
        next_cursor?: string
      }
      if (
        !isCurrent(value, route) ||
        signal.aborted ||
        requestedValues !== valuesPage.key()
      )
        return false
      const committed = request.commit(() => {
        active = loaded
        activeValues = values.values
        valuesPage.state.next = values.next_cursor ?? ""
        if (!values.next_cursor) allValuesLoaded = true
        const existing = new Set(drafts.map((item) => item.id))
        drafts = [
          ...drafts,
          ...activeValues
            .filter((item) => !existing.has(item.id))
            .map((item) => ({
              id: item.id,
              attribute: item.attribute,
              value: item.value,
              sensitive: item.sensitive,
            })),
        ]
      })
      if (!committed) return false
      return (
        !refreshTitle ||
        route.kind !== "project-record" ||
        (await loadReferences(route, value, signal))
      )
    } catch {
      if (isCurrent(value, route) && !signal.aborted)
        request.fail(
          new Error("Record metadata and values could not be loaded."),
        )
      return false
    } finally {
      request.finish()
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
    return drafts.filter(
      (draft) =>
        draft.attribute === id &&
        !draft.removed &&
        (currentRoute.kind === "project-record-new" ||
          !draft.id ||
          activeValues.some((value) => value.id === draft.id)),
    )
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
      position === index
        ? {
            ...draft,
            ...patch,
            changed: draft.changed || "value" in patch || "sensitive" in patch,
          }
        : draft,
    )
  }
  function draftIndex(draft: ValueDraft): number {
    return drafts.indexOf(draft)
  }
  function typedValue(attribute: ProjectRecordAttribute, raw: string): unknown {
    return attribute.type === "number"
      ? raw === ""
        ? ""
        : Number(raw)
      : attribute.type === "datetime"
        ? dateTimeValue(raw)
        : raw
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
    const lines = attributes
      .filter((attribute) => attribute.display === "primary")
      .flatMap((attribute) =>
        recordValues
          .filter((value) => value.attribute === attribute.id)
          .flatMap((value) => valueLines(attribute, value)),
      )
    return lines.length ? lines : [`Untitled ${schema?.label ?? "record"}`]
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
  async function loadMoreIncomingReferences(): Promise<boolean> {
    const route = currentRoute
    if (route.kind !== "project-record") return false
    return loadReferences(route, generation, abortController!.signal)
  }
  async function loadReferences(
    route: Extract<RecordsRoute, { recordID: string }>,
    value: number,
    signal: AbortSignal,
  ): Promise<boolean> {
    const request = referencesLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    try {
      const response = await fetchProjectRecordIncomingReferences(
        route.workspaceID,
        route.projectID,
        route.schemaID,
        route.recordID,
        incomingPage.params().cursor || undefined,
        request.signal,
      )
      if (!isCurrent(value, route) || !request.current()) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (!response.ok) throw new Error()
      const next = (await response.json()) as {
        groups: ProjectRecordIncomingReferenceGroup[]
        next_cursor?: string
      }
      return request.commit(() => {
        incomingReferences = next.groups
        incomingPage.state.next = next.next_cursor ?? ""
      })
    } catch {
      request.fail(new Error("Reverse references could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  async function saveSchema(): Promise<void> {
    if (saving) return
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
        `${schemaPath(route.workspaceID, route.projectID, saved.id)}/overview`,
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
    if (saving || !schemaLoad.state.hasLoaded) return
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
      resetAttributeForm()
      runtime.navigate(
        `${schemaPath(route.workspaceID, route.projectID, route.schemaID)}/fields`,
      )
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
    if (
      !isSchemaRoute(route) ||
      !window.confirm(
        `Remove ${attribute.label}? Its stored values will be removed from records.`,
      )
    )
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
    if (saving || !schemaLoad.state.hasLoaded) return
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
              attribute: attributeFor(attribute)?.name ?? attribute,
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
              attribute: attributeFor(attribute)?.name ?? attribute,
              value,
              sensitive,
            })),
          update: drafts
            .filter(
              (draft) =>
                draft.id !== undefined && !draft.removed && draft.changed,
            )
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
                >{workspace.name ?? "Workspace"}</RouterLink
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
          {#if projectLoad.reloading || schemasLoad.reloading || schemaLoad.reloading || recordsLoad.reloading || recordLoad.reloading}<UI.Text
              role="status">Refreshing records…</UI.Text
            >{/if}
          {#if projectLoad.state.error || schemasLoad.state.error || schemaLoad.state.error || recordsLoad.state.error || recordLoad.state.error}<UI.Notice
              variant="destructive"
              role="alert"
              >{projectLoad.state.error ||
                schemasLoad.state.error ||
                schemaLoad.state.error ||
                recordsLoad.state.error ||
                recordLoad.state.error}<UI.Button
                variant="outline"
                onclick={() =>
                  void loadRoute(
                    currentRoute,
                    generation,
                    abortController!.signal,
                  )}>Retry</UI.Button
              ></UI.Notice
            >{/if}
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
          {:else if isSchemaRoute(currentRoute) && schemaLoad.status !== "ready"}
            {#if schemaLoad.loading}<UI.Text role="status"
                >Loading record type…</UI.Text
              >{/if}
          {:else if currentRoute.kind === "project-record-schema-overview"}
            {#if schema}
              <UI.AdminHeader
                kind="Record type"
                title={schema.label}
                description={schema.description}
              >
                {#snippet actions()}<UI.RouterLink
                    variant="outline"
                    href={`${schemaPath(workspace.id, currentRoute.projectID, currentRoute.schemaID)}/edit`}
                    >Edit type</UI.RouterLink
                  >{/snippet}
              </UI.AdminHeader>
              <UI.DetailSection title="Type information"
                ><UI.Property label="Name">{schema.name}</UI.Property
                ><UI.Property label="Created"
                  >{new Date(schema.created_at).toLocaleString()}</UI.Property
                ></UI.DetailSection
              >
              <UI.NavigationGrid>
                <UI.NavigationCard
                  title="Records"
                  description="Search and work with records of this type."
                  href={schemaPath(
                    workspace.id,
                    currentRoute.projectID,
                    schema.id,
                  )}
                />
                <UI.NavigationCard
                  title="Fields"
                  description="Configure value types, references, constraints, and display."
                  href={`${schemaPath(workspace.id, currentRoute.projectID, schema.id)}/fields`}
                />
              </UI.NavigationGrid>
            {:else if schemaLoad.loading}<UI.Text role="status"
                >Loading record type…</UI.Text
              >{/if}
          {:else if currentRoute.kind === "project-record-fields"}
            <UI.AdminHeader
              title={`${schema?.label ?? "Record type"} fields`}
              description="Configure the fields used by this record type."
            >
              {#snippet actions()}<UI.RouterLink
                  variant="default"
                  href={`${schemaPath(workspace.id, currentRoute.projectID, currentRoute.schemaID)}/fields/new`}
                  >New field</UI.RouterLink
                >{/snippet}
            </UI.AdminHeader>
            <UI.SearchToolbar
              label="Search fields"
              placeholder="Field name, label, or type"
              bind:value={fieldsPage.state.search}
              loading={fieldsPage.state.busy}
              onsearch={() => void fieldsPage.search(reloadFields)}
            />
            <UI.RecordTable
              headers={[
                "Field",
                "Type",
                "Values",
                "Uniqueness",
                "Display",
                "Actions",
              ]}
            >
              {#each attributes as attribute (attribute.id)}<UI.RecordRow>
                  <UI.RecordCell primary
                    ><UI.RouterLink
                      href={`${schemaPath(workspace.id, currentRoute.projectID, currentRoute.schemaID)}/fields/${encodeURIComponent(attribute.id)}/edit`}
                      >{attribute.label}</UI.RouterLink
                    ><UI.Text size="small" tone="muted"
                      >{attribute.name}</UI.Text
                    ></UI.RecordCell
                  >
                  <UI.RecordCell>{attribute.type}</UI.RecordCell><UI.RecordCell
                    >{attribute.cardinality}</UI.RecordCell
                  ><UI.RecordCell>{attribute.uniqueness}</UI.RecordCell
                  ><UI.RecordCell
                    >{attribute.display} · {attribute.display_order}</UI.RecordCell
                  >
                  <UI.RecordCell
                    ><UI.Button
                      variant="outline"
                      size="sm"
                      disabled={saving}
                      onclick={() => void deleteAttribute(attribute)}
                      >Remove</UI.Button
                    ></UI.RecordCell
                  >
                </UI.RecordRow>{:else}<UI.TableEmpty
                  columns={6}
                  message={schemaLoad.state.error
                    ? "Fields could not be loaded."
                    : schema
                      ? "No fields yet."
                      : "Loading fields…"}
                />{/each}
            </UI.RecordTable>
            <UI.PaginationFooter
              noun="fields"
              count={attributes.length}
              previous={fieldsPage.state.history.length > 0}
              next={!!fieldsPage.state.next}
              loading={fieldsPage.state.busy || saving}
              onprevious={() => void fieldsPage.previous(reloadFields)}
              onnext={() => void fieldsPage.next(reloadFields)}
            />
          {:else if currentRoute.kind === "project-record-field-new" || currentRoute.kind === "project-record-field-edit"}
            {#if !schema && schemaLoad.loading}<UI.Text role="status"
                >Loading record type…</UI.Text
              >{/if}
          {:else if currentRoute.kind === "project-record-schema-new" || currentRoute.kind === "project-record-schema-edit"}{#snippet schemaActions()}{#if currentRoute.kind === "project-record-schema-edit"}<UI.Button
                  variant="outline"
                  type="button"
                  disabled={saving}
                  onclick={() => void deleteSchema()}
                  >Remove record type</UI.Button
                >{/if}{/snippet}
            <UI.AdminHeader
              title={currentRoute.kind === "project-record-schema-new"
                ? "New record type"
                : `Edit ${schema?.label ?? "record type"}`}
              actions={schemaActions}
            />
            <UI.Form
              surface
              onsubmit={(event) => {
                event.preventDefault()
                void saveSchema()
              }}
            >
              <UI.FormSection
                title="Identity"
                description="The technical name is permanent; the label is shown to users."
                ><UI.Field>
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
              </UI.FormSection><UI.FormSection title="Description"
                ><UI.Field>
                  <UI.Label for="schema-description">Description</UI.Label>
                  <UI.Textarea
                    id="schema-description"
                    rows={3}
                    maxlength={4096}
                    bind:value={schemaDescription}
                  ></UI.Textarea>
                </UI.Field>
              </UI.FormSection>{#if error !== ""}<UI.Text
                  as="p"
                  size="small"
                  role="alert">{error}</UI.Text
                >{/if}
              <UI.FormActions>
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
              </UI.FormActions>
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
                      >{#each primaryLines(active?.values ?? activeValues) as line}<UI.Text
                          >{line}</UI.Text
                        >{/each}</UI.Stack
                    >{/if}
                </UI.Heading>
              </PageHeading>
              {#if active}<UI.DetailSection title="Record information"
                  ><UI.Property label="Created"
                    >{new Date(active.created_at).toLocaleString()}</UI.Property
                  ></UI.DetailSection
                >{/if}
              <UI.DetailSection title="Record values">
                {#if recordsStatus === "checking"}<UI.Property label="Values"
                    >Loading record…</UI.Property
                  >{:else if activeValues.length === 0}<UI.Property
                    label="Values">No values on this page.</UI.Property
                  >{:else}{#each attributes as attribute (attribute.id)}{@const items =
                      activeValues.filter(
                        (item) => item.attribute === attribute.id,
                      )}{#if items.length !== 0}<UI.Property
                        label={attribute.label}
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
                      </UI.Property>{/if}{/each}{/if}
              </UI.DetailSection>
              <UI.PaginationFooter
                noun="values"
                count={activeValues.length}
                previous={valuesPage.state.history.length > 0}
                next={!!valuesPage.state.next}
                loading={valuesPage.state.busy || recordLoad.state.pending}
                onprevious={() => void valuesPage.previous(reloadValues)}
                onnext={() => void valuesPage.next(reloadValues)}
              />
              {#if referencesLoad.state.error}<UI.Notice
                  variant="destructive"
                  role="alert"
                  >{referencesLoad.state.error}<UI.Button
                    variant="outline"
                    onclick={() => void loadMoreIncomingReferences()}
                    >Retry references</UI.Button
                  ></UI.Notice
                >{/if}
              {#if incomingReferencesLoading}<UI.Text role="status"
                  >Loading references…</UI.Text
                >{/if}
              {#if incomingReferences.length !== 0 || incomingPage.state.history.length > 0}<UI.Stack
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
                  <UI.PaginationFooter
                    noun="references"
                    count={incomingReferences.reduce(
                      (count, group) => count + group.references.length,
                      0,
                    )}
                    previous={incomingPage.state.history.length > 0}
                    next={!!incomingPage.state.next}
                    loading={incomingReferencesLoading ||
                      incomingPage.state.busy}
                    onprevious={() =>
                      void incomingPage.previous(loadMoreIncomingReferences)}
                    onnext={() =>
                      void incomingPage.next(loadMoreIncomingReferences)}
                  />
                </UI.Stack>{/if}
            </UI.Stack>
          {:else if currentRoute.kind === "project-record-new" || currentRoute.kind === "project-record-edit"}{#snippet recordActions()}{#if currentRoute.kind === "project-record-edit"}<UI.Button
                  variant="outline"
                  type="button"
                  disabled={saving}
                  onclick={() => void deleteRecord()}>Remove record</UI.Button
                >{/if}{/snippet}
            <UI.AdminHeader
              kind={schema?.label ?? "Record"}
              title={currentRoute.kind === "project-record-new"
                ? "New record"
                : "Edit record"}
              actions={recordActions}
            />
            <UI.Form
              surface
              onsubmit={(event) => {
                event.preventDefault()
                void saveRecord()
              }}
            >
              {#if recordsStatus === "checking"}<UI.Text as="p" tone="muted">
                  Loading record...
                </UI.Text>{:else if attributes.length === 0}<UI.Text
                  as="p"
                  tone="muted"
                >
                  Add fields to this record type before creating records.
                </UI.Text>{:else}{#each attributes as attribute (attribute.id)}<UI.FormSection
                    title={attribute.label}
                    description={attribute.description}
                  >
                    {#each attributeValues(attribute.id) as draft (draft.id ?? `${draft.attribute}-${draftIndex(draft)}`)}{@const index =
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
                              >{:else if attribute.type === "file"}<ProjectFilePicker
                                workspaceID={workspace.id}
                                projectID={currentRoute.projectID}
                                label={attribute.label}
                                bind:value={
                                  () => valueLabel(draft.value),
                                  (value) =>
                                    (drafts = setRecordReference(
                                      drafts,
                                      index,
                                      value,
                                    ))
                                }
                              />
                            {:else if attribute.type === "record"}<ProjectRecordPicker
                                workspaceID={workspace.id}
                                projectID={currentRoute.projectID}
                                schemaID={attribute.target_schema ?? ""}
                                label={attribute.label}
                                bind:value={
                                  () => valueLabel(draft.value),
                                  (value) =>
                                    (drafts = setRecordReference(
                                      drafts,
                                      index,
                                      value,
                                    ))
                                }
                              />
                            {:else}<UI.Label
                                for={`value-${attribute.id}-${index}`}
                                >{attribute.type === "datetime"
                                  ? "Date and time (local)"
                                  : "Value"}</UI.Label
                              ><UI.Input
                                id={`value-${attribute.id}-${index}`}
                                required
                                step={attribute.type === "number" ||
                                attribute.type === "datetime"
                                  ? "any"
                                  : undefined}
                                value={attribute.type === "datetime"
                                  ? dateTimeInput(draft.value)
                                  : valueLabel(draft.value)}
                                type={attribute.type === "number"
                                  ? "number"
                                  : attribute.type === "datetime"
                                    ? "datetime-local"
                                    : "text"}
                                placeholder={attribute.type === "text"
                                  ? "Enter text"
                                  : undefined}
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
                      </UI.Stack>{/each}
                    {#each drafts.filter((draft) => draft.attribute === attribute.id && draft.removed) as draft}<UI.Row
                        ><UI.Text tone="muted"
                          >Value marked for removal.</UI.Text
                        ><UI.Button
                          variant="outline"
                          onclick={() =>
                            setDraft(draftIndex(draft), { removed: false })}
                          >Undo removal</UI.Button
                        ></UI.Row
                      >{/each}
                    {#if attribute.cardinality === "many" || !drafts.some((draft) => draft.attribute === attribute.id)}<UI.Button
                        variant="outline"
                        size="sm"
                        type="button"
                        disabled={currentRoute.kind === "project-record-edit" &&
                          attribute.cardinality === "one" &&
                          !allValuesLoaded}
                        onclick={() =>
                          (drafts = [...drafts, emptyDraft(attribute)])}
                        >{attribute.cardinality === "many"
                          ? "Add value"
                          : "Set value"}</UI.Button
                      >{/if}
                    {#if currentRoute.kind === "project-record-edit" && attribute.cardinality === "one" && !allValuesLoaded && !drafts.some((draft) => draft.attribute === attribute.id)}<UI.Text
                        size="small"
                        tone="muted"
                        >Browse the remaining value pages to check whether this
                        field is already set.</UI.Text
                      >{/if}
                  </UI.FormSection>{/each}{/if}
              {#if currentRoute.kind === "project-record-edit"}<UI.PaginationFooter
                  noun="values"
                  count={activeValues.length}
                  previous={valuesPage.state.history.length > 0}
                  next={!!valuesPage.state.next}
                  loading={valuesPage.state.busy || recordLoad.state.pending}
                  onprevious={() => void valuesPage.previous(reloadValues)}
                  onnext={() => void valuesPage.next(reloadValues)}
                />{/if}
              {#if error !== ""}<UI.Text as="p" size="small" role="alert">
                  {error}
                </UI.Text>{/if}
              <UI.FormActions>
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
              </UI.FormActions>
            </UI.Form>
          {:else if currentRoute.kind === "project-record-schema"}{#snippet schemaListActions()}<UI.Button
                variant="outline"
                size="sm"
                type="button"
                onclick={() =>
                  runtime.navigate(
                    `${schemaPath(workspace.id, currentRoute.projectID, currentRoute.schemaID)}/overview`,
                  )}>Type overview</UI.Button
              ><UI.Button
                variant="default"
                size="sm"
                type="button"
                onclick={() =>
                  runtime.navigate(
                    `${schemaPath(workspace.id, currentRoute.projectID, currentRoute.schemaID)}/new`,
                  )}>New record</UI.Button
              >{/snippet}
            <UI.AdminHeader
              title={`${schema?.label ?? "Type"} records`}
              description={schema?.description}
              actions={schemaListActions}
            />
            <UI.SearchToolbar
              label="Search records"
              placeholder="Record display text"
              bind:value={recordsPage.state.search}
              loading={recordsPage.state.busy || recordsLoad.state.pending}
              onsearch={() => void recordsPage.search(reloadRecords)}
            />
            {#if recordsStatus === "checking"}<UI.Text as="p" tone="muted">
                Loading records...
              </UI.Text>{:else if recordsStatus === "unavailable"}<UI.Text
                as="p"
                tone="muted"
              >
                Records could not be loaded.
              </UI.Text>{/if}
            <UI.RecordTable
              headers={[
                "Record",
                ...displayAttributes
                  .filter((attribute) => attribute.display === "secondary")
                  .map((attribute) => attribute.label),
              ]}
            >
              {#each records as record (record.id)}<UI.RecordRow>
                  <UI.RecordCell primary
                    ><RouterLink
                      href={recordPath(
                        workspace.id,
                        currentRoute.projectID,
                        currentRoute.schemaID,
                        record.id,
                      )}
                      >{primaryLines(record.values ?? []).join(
                        " · ",
                      )}</RouterLink
                    ></UI.RecordCell
                  >
                  {#each displayAttributes.filter((attribute) => attribute.display === "secondary") as attribute (attribute.id)}<UI.RecordCell
                    >
                      <UI.Stack gap="tight"
                        >{#each (record.values ?? []).filter((item) => item.attribute === attribute.id) as item}{@const href =
                            referencePath(attribute, item)}{#if href}<RouterLink
                              {href}
                              >{valueLines(attribute, item).join(
                                " · ",
                              )}</RouterLink
                            >{:else}<UI.Text
                              >{valueLines(attribute, item).join(
                                " · ",
                              )}</UI.Text
                            >{/if}{:else}<UI.Text tone="muted">—</UI.Text
                          >{/each}</UI.Stack
                      >
                    </UI.RecordCell>{/each}
                </UI.RecordRow>{:else}<UI.TableEmpty
                  columns={1 +
                    displayAttributes.filter(
                      (attribute) => attribute.display === "secondary",
                    ).length}
                  message={recordsStatus === "unavailable"
                    ? "Records could not be loaded."
                    : recordsStatus === "checking"
                      ? "Loading records…"
                      : "No matching records."}
                />{/each}
            </UI.RecordTable>
            <UI.PaginationFooter
              count={records.length}
              previous={recordsPage.state.history.length > 0}
              next={!!recordsPage.state.next}
              loading={recordsPage.state.busy || recordsLoad.state.pending}
              onprevious={() => void recordsPage.previous(reloadRecords)}
              onnext={() => void recordsPage.next(reloadRecords)}
            />
          {:else}{#snippet recordTypesActions()}<UI.Button
                variant="default"
                type="button"
                onclick={() =>
                  runtime.navigate(
                    `${recordsPath(workspace.id, currentRoute.projectID)}/new`,
                  )}>New record type</UI.Button
              >{/snippet}
            <UI.AdminHeader
              title="Record types"
              description="Define reusable record types and their fields."
              actions={recordTypesActions}
            />
            <UI.SearchToolbar
              label="Search record types"
              placeholder="Name, label, or description"
              bind:value={typesPage.state.search}
              loading={typesPage.state.busy || schemasLoad.state.pending}
              onsearch={() => void typesPage.search(reloadTypes)}
            />
            {#if schemasStatus === "checking"}<UI.Text as="p" tone="muted">
                Loading record types...
              </UI.Text>{:else if schemasStatus === "unavailable"}<UI.Text
                as="p"
                tone="muted"
              >
                Record types could not be loaded.
              </UI.Text>{/if}
            <UI.RecordTable headers={["Type", "Description", "Created"]}>
              {#each schemas as item (item.id)}<UI.RecordRow>
                  <UI.RecordCell primary
                    ><RouterLink
                      href={`${schemaPath(workspace.id, currentRoute.projectID, item.id)}/overview`}
                      >{item.label}</RouterLink
                    ><UI.Text size="small" tone="muted">{item.name}</UI.Text
                    ></UI.RecordCell
                  >
                  <UI.RecordCell>{item.description || "—"}</UI.RecordCell
                  ><UI.RecordCell
                    >{new Date(item.created_at).toLocaleString()}</UI.RecordCell
                  >
                </UI.RecordRow>{:else}<UI.TableEmpty
                  columns={3}
                  message={schemasStatus === "unavailable"
                    ? "Record types could not be loaded."
                    : schemasStatus === "checking"
                      ? "Loading types…"
                      : "No matching record types."}
                />{/each}
            </UI.RecordTable>
            <UI.PaginationFooter
              noun="types"
              count={schemas.length}
              previous={typesPage.state.history.length > 0}
              next={!!typesPage.state.next}
              loading={typesPage.state.busy || schemasLoad.state.pending}
              onprevious={() => void typesPage.previous(reloadTypes)}
              onnext={() => void typesPage.next(reloadTypes)}
            />
          {/if}
          {#if (currentRoute.kind === "project-record-field-new" || currentRoute.kind === "project-record-field-edit") && schema !== null}
            <UI.Stack as="section">
              <UI.AdminHeader
                kind={schema.label}
                title={attributeForm.id === undefined
                  ? "New field"
                  : `Edit ${attributeForm.label}`}
              />
              <UI.Form
                surface
                onsubmit={(event) => {
                  event.preventDefault()
                  void saveAttribute()
                }}
              >
                <UI.FormSection
                  title="Identity"
                  description="The technical name is permanent; the label is shown to users."
                >
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
                </UI.FormSection><UI.FormSection
                  title="Value type"
                  description="Choose the kind of value and any referenced resource type."
                  ><UI.Field>
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
                  {#if attributeForm.type === "record"}<ProjectRecordTypePicker
                      workspaceID={workspace.id}
                      projectID={currentRoute.projectID}
                      bind:value={attributeForm.target_schema}
                    />{/if}
                </UI.FormSection><UI.FormSection
                  title="Constraints"
                  description="Configure cardinality and uniqueness."
                >
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
                </UI.FormSection><UI.FormSection
                  title="Display"
                  description="Choose how values identify records and appear in directories."
                  ><UI.Field>
                    <UI.Label for="attribute-display"
                      >Directory display</UI.Label
                    >
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
                </UI.FormSection><UI.FormSection title="Help text">
                  <UI.Field>
                    <UI.Label for="attribute-description">Description</UI.Label
                    ><UI.Textarea
                      id="attribute-description"
                      rows={2}
                      bind:value={attributeForm.description}
                    ></UI.Textarea>
                  </UI.Field>
                </UI.FormSection><UI.FormActions>
                  <UI.RouterLink
                    variant="outline"
                    href={`${schemaPath(workspace.id, currentRoute.projectID, currentRoute.schemaID)}/fields`}
                    >Cancel</UI.RouterLink
                  ><UI.Button variant="default" type="submit" disabled={saving}
                    >{saving ? "Saving..." : "Save field"}</UI.Button
                  >
                </UI.FormActions>
              </UI.Form>
            </UI.Stack>
          {/if}
          {#if error}<UI.Notice variant="destructive" role="alert"
              >{error}</UI.Notice
            >{/if}
        </PageBody></SidebarPage.Body
      >
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
