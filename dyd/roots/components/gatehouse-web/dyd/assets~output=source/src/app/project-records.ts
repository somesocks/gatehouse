export type RecordAuthor = { principal?: { id: string; name?: string }; agent?: { id: string; label?: string }; gateway?: string }

export type ProjectRecordSchema = { id: string; name: string; label: string; description: string; author: RecordAuthor; created_at: string }
export type ProjectRecordAttribute = { id: string; name: string; label: string; description: string; type: "text" | "number" | "boolean" | "datetime" | "record"; target_schema?: string; cardinality: "one" | "many"; uniqueness: "none" | "record" | "global"; display: "none" | "primary" | "secondary"; display_order: number; author: RecordAuthor; created_at: string }
export type ProjectRecordReferenceDisplay = { schema_label: string; primary_values: { value: unknown; sensitive: boolean }[] }
export type ProjectRecordValue = { id: string; attribute: string; value: unknown; sensitive: boolean; author: RecordAuthor; created_at: string; reference?: ProjectRecordReferenceDisplay }
export type ProjectRecord = { id: string; author: RecordAuthor; created_at: string; values?: ProjectRecordValue[] }
export type ProjectRecordsResponse = { records: ProjectRecord[]; next_cursor?: string }
export type ProjectRecordValuesResponse = { values: ProjectRecordValue[]; next_cursor?: string }
export type ProjectRecordIncomingReference = { id: string; primary_values: { value: unknown; sensitive: boolean }[] }
export type ProjectRecordIncomingReferenceGroup = { source_schema: { id: string; label: string }; source_attribute: { id: string; name: string; label: string }; references: ProjectRecordIncomingReference[] }
export type ProjectRecordIncomingReferencesResponse = { groups: ProjectRecordIncomingReferenceGroup[]; next_cursor?: string }
export type ProjectRecordValueCreate = { attribute: string; value: unknown; sensitive: boolean }
export type ProjectRecordValueUpdate = { id: string; value: unknown; sensitive?: boolean }

export function projectRecordSchemasAPIPath(workspaceID: string, projectID: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(workspaceID)}/projects/${encodeURIComponent(projectID)}/record-schemas`
}

export function projectRecordsAPIPath(workspaceID: string, projectID: string, schemaID: string): string {
  return `${projectRecordSchemasAPIPath(workspaceID, projectID)}/${encodeURIComponent(schemaID)}/records`
}

function request(path: string, init: RequestInit = {}, signal?: AbortSignal): Promise<Response> {
  return fetch(path, { credentials: "same-origin", ...init, ...(signal === undefined ? {} : { signal }) })
}

export const fetchProjectRecordSchemas = (workspaceID: string, projectID: string, signal?: AbortSignal) => request(projectRecordSchemasAPIPath(workspaceID, projectID), {}, signal)
export const fetchProjectRecordSchema = (workspaceID: string, projectID: string, schemaID: string, signal?: AbortSignal) => request(`${projectRecordSchemasAPIPath(workspaceID, projectID)}/${encodeURIComponent(schemaID)}`, {}, signal)
export const createProjectRecordSchema = (workspaceID: string, projectID: string, input: Pick<ProjectRecordSchema, "name" | "label" | "description">, signal?: AbortSignal) => request(projectRecordSchemasAPIPath(workspaceID, projectID), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) }, signal)
export const updateProjectRecordSchema = (workspaceID: string, projectID: string, schemaID: string, input: Pick<ProjectRecordSchema, "label" | "description">, signal?: AbortSignal) => request(`${projectRecordSchemasAPIPath(workspaceID, projectID)}/${encodeURIComponent(schemaID)}`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) }, signal)
export const removeProjectRecordSchema = (workspaceID: string, projectID: string, schemaID: string, signal?: AbortSignal) => request(`${projectRecordSchemasAPIPath(workspaceID, projectID)}/${encodeURIComponent(schemaID)}`, { method: "DELETE" }, signal)

export const fetchProjectRecordAttributes = (workspaceID: string, projectID: string, schemaID: string, signal?: AbortSignal) => request(`${projectRecordSchemasAPIPath(workspaceID, projectID)}/${encodeURIComponent(schemaID)}/attributes`, {}, signal)
export const createProjectRecordAttribute = (workspaceID: string, projectID: string, schemaID: string, input: Omit<ProjectRecordAttribute, "id" | "author" | "created_at">, signal?: AbortSignal) => request(`${projectRecordSchemasAPIPath(workspaceID, projectID)}/${encodeURIComponent(schemaID)}/attributes`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) }, signal)
export const updateProjectRecordAttribute = (workspaceID: string, projectID: string, schemaID: string, attributeID: string, input: Omit<ProjectRecordAttribute, "id" | "name" | "author" | "created_at">, signal?: AbortSignal) => request(`${projectRecordSchemasAPIPath(workspaceID, projectID)}/${encodeURIComponent(schemaID)}/attributes/${encodeURIComponent(attributeID)}`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) }, signal)
export const removeProjectRecordAttribute = (workspaceID: string, projectID: string, schemaID: string, attributeID: string, signal?: AbortSignal) => request(`${projectRecordSchemasAPIPath(workspaceID, projectID)}/${encodeURIComponent(schemaID)}/attributes/${encodeURIComponent(attributeID)}`, { method: "DELETE" }, signal)

export const fetchProjectRecords = (workspaceID: string, projectID: string, schemaID: string, signal?: AbortSignal) => request(projectRecordsAPIPath(workspaceID, projectID, schemaID), {}, signal)
export const fetchProjectRecord = (workspaceID: string, projectID: string, schemaID: string, recordID: string, signal?: AbortSignal) => request(`${projectRecordsAPIPath(workspaceID, projectID, schemaID)}/${encodeURIComponent(recordID)}`, {}, signal)
export const createProjectRecord = (workspaceID: string, projectID: string, schemaID: string, values: ProjectRecordValueCreate[], signal?: AbortSignal) => request(projectRecordsAPIPath(workspaceID, projectID, schemaID), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ values }) }, signal)
export const removeProjectRecord = (workspaceID: string, projectID: string, schemaID: string, recordID: string, signal?: AbortSignal) => request(`${projectRecordsAPIPath(workspaceID, projectID, schemaID)}/${encodeURIComponent(recordID)}`, { method: "DELETE" }, signal)
export const fetchProjectRecordValues = (workspaceID: string, projectID: string, schemaID: string, recordID: string, signal?: AbortSignal) => request(`${projectRecordsAPIPath(workspaceID, projectID, schemaID)}/${encodeURIComponent(recordID)}/values`, {}, signal)
export const fetchProjectRecordIncomingReferences = (workspaceID: string, projectID: string, schemaID: string, recordID: string, cursor?: string, signal?: AbortSignal) => request(`${projectRecordsAPIPath(workspaceID, projectID, schemaID)}/${encodeURIComponent(recordID)}/references${cursor === undefined ? "" : `?cursor=${encodeURIComponent(cursor)}`}`, {}, signal)
export const mutateProjectRecordValues = (workspaceID: string, projectID: string, schemaID: string, recordID: string, input: { create: ProjectRecordValueCreate[]; update: ProjectRecordValueUpdate[]; delete: { id: string }[] }, signal?: AbortSignal) => request(`${projectRecordsAPIPath(workspaceID, projectID, schemaID)}/${encodeURIComponent(recordID)}/values/mutate`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(input) }, signal)
