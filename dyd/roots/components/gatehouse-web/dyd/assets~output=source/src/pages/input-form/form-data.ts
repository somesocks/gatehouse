export type InputField = {
  id?: string
  label?: string
  type: "text" | "number" | "boolean" | "options" | "object" | "list" | "files"
  optional?: boolean
  min_length?: string
  max_length?: string
  min?: string
  max?: string
  integer?: boolean
  choices?: string[]
  fields?: InputField[]
  item?: InputField
  max_files?: string
  media_types?: string[]
}

export type InputFileSummary = { id: string; name: string; size: string | number; media_type?: string }

export type InputForm = {
  version: string
  type: "form"
  title: string
  fields: InputField[]
}

export type InputDocument = { form: InputForm; draft: Record<string, unknown> }

// JSON.parse coerces large JSON numbers to imprecise JS numbers. Keep their
// original text in both form constraints and drafts; serialize numeric fields
// using their descriptor when patching a field or a whole list.
export function parseInputDocument(source: string): InputDocument {
  let protectedJSON = ""
  let quoted = false
  let escaped = false
  for (let index = 0; index < source.length; ) {
    const character = source[index]
    if (quoted) {
      protectedJSON += character
      if (escaped) escaped = false
      else if (character === "\\") escaped = true
      else if (character === '"') quoted = false
      index += 1
    } else if (character === '"') {
      quoted = true
      protectedJSON += character
      index += 1
    } else if (character === "-" || (character >= "0" && character <= "9")) {
      const number = /^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?/.exec(
        source.slice(index),
      )
      if (number === null) throw new Error("invalid JSON number")
      protectedJSON += JSON.stringify(number[0])
      index += number[0].length
    } else {
      protectedJSON += character
      index += 1
    }
  }
  const document = JSON.parse(protectedJSON) as InputDocument
  if (
    document?.form?.version !== "1" ||
    document.form.type !== "form" ||
    !Array.isArray(document.form.fields) ||
    document.draft === null ||
    typeof document.draft !== "object" ||
    Array.isArray(document.draft)
  )
    throw new Error("unsupported input form")
  return document
}

const numberPattern = /^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?$/

export function encodeFieldValue(field: InputField, value: unknown): string {
  switch (field.type) {
    case "text":
    case "options":
      if (typeof value !== "string") break
      return JSON.stringify(value)
    case "number":
      if (typeof value !== "string" || !numberPattern.test(value)) break
      return value
    case "boolean":
      if (typeof value !== "boolean") break
      return String(value)
    case "files":
      if (!Array.isArray(value) || value.some((file) =>
        file === null || typeof file !== "object" || typeof file.id !== "string" ||
        typeof file.name !== "string" || !/^(0|[1-9]\d*)$/.test(String(file.size)) ||
        file.media_type !== undefined && typeof file.media_type !== "string")) break
      return `[${value.map((file: InputFileSummary) =>
        `{"id":${JSON.stringify(file.id)},"name":${JSON.stringify(file.name)},"size":${file.size}${file.media_type === undefined ? "" : `,"media_type":${JSON.stringify(file.media_type)}`}}`,
      ).join(",")}]`
    case "object": {
      if (value === null || typeof value !== "object" || Array.isArray(value))
        break
      const entries = Object.entries(value).map(([key, child]) => {
        const descriptor = field.fields?.find((candidate) => candidate.id === key)
        if (descriptor === undefined) throw new Error(`Unknown field ${key}`)
        return `${JSON.stringify(key)}:${encodeFieldValue(descriptor, child)}`
      })
      return `{${entries.join(",")}}`
    }
    case "list":
      if (!Array.isArray(value) || field.item === undefined) break
      return `[${value.map((item) => encodeFieldValue(field.item!, item)).join(",")}]`
  }
  throw new Error("Enter a valid answer before saving.")
}

export function patchDraft(
  draft: Record<string, unknown>,
  path: string[],
  value?: unknown,
  remove = false,
): Record<string, unknown> {
  if (path.length === 0) throw new Error("empty field path")
  const result = { ...draft }
  let parent = result
  for (const segment of path.slice(0, -1)) {
    const current = Object.hasOwn(parent, segment) ? parent[segment] : undefined
    const nested =
      current !== null && typeof current === "object" && !Array.isArray(current)
        ? { ...(current as Record<string, unknown>) }
        : {}
    Object.defineProperty(parent, segment, { value: nested, enumerable: true, writable: true, configurable: true })
    parent = nested
  }
  if (remove) delete parent[path[path.length - 1]]
  else Object.defineProperty(parent, path[path.length - 1], { value, enumerable: true, writable: true, configurable: true })
  return result
}

export function fieldValue(draft: Record<string, unknown>, path: string[]): unknown {
  let current: unknown = draft
  for (const segment of path) {
    if (current === null || typeof current !== "object" || Array.isArray(current))
      return undefined
    if (!Object.hasOwn(current, segment)) return undefined
    current = (current as Record<string, unknown>)[segment]
  }
  return current
}

export function initialListValue(field: InputField): unknown {
  switch (field.type) {
    case "object":
      return {}
    case "list":
      return []
    case "boolean":
      return false
    case "options":
      return field.choices?.[0] ?? ""
    case "text":
      return " ".repeat(Math.max(0, Number(field.min_length ?? "0")))
    case "number":
      return field.min ?? (field.max !== undefined && field.max.startsWith("-") ? field.max : "0")
    case "files":
      return []
  }
}

// An ordinary unchecked required checkbox is false, not unanswered. Apply
// those defaults to the stored draft immediately before Submit, including
// checkboxes inside repeated objects.
export function requiredBooleanDefaults(
  form: InputForm,
  draft: Record<string, unknown>,
): { path: string[]; value: unknown }[] {
  const updates: { path: string[]; value: unknown }[] = []
  function visit(fields: InputField[], object: Record<string, unknown>, path: string[]): void {
    for (const field of fields) {
      if (field.id === undefined) continue
      const fieldPath = [...path, field.id]
      const current = Object.hasOwn(object, field.id) ? object[field.id] : undefined
      if (field.type === "boolean" && !field.optional && current === undefined)
        updates.push({ path: fieldPath, value: false })
      else if (field.type === "object" && !field.optional && current === undefined)
        visit(field.fields ?? [], {}, fieldPath)
      else if (field.type === "object" && isObject(current))
        visit(field.fields ?? [], current, fieldPath)
      else if (field.type === "list" && Array.isArray(current) && field.item !== undefined) {
        const entries = current.map((item) => fillListBooleans(field.item!, item))
        if (entries.some((item, index) => item !== current[index]))
          updates.push({ path: fieldPath, value: entries })
      }
    }
  }
  visit(form.fields, draft, [])
  return updates
}

function isObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
}

function fillListBooleans(field: InputField, value: unknown): unknown {
  if (field.type === "object" && isObject(value)) {
    let copy: Record<string, unknown> | null = null
    for (const child of field.fields ?? []) {
      if (child.id === undefined) continue
      const current = Object.hasOwn(value, child.id) ? value[child.id] : undefined
      if (child.type === "boolean" && !child.optional && current === undefined) {
        copy ??= { ...value }
        Object.defineProperty(copy, child.id, { value: false, enumerable: true, writable: true, configurable: true })
      } else if (current !== undefined) {
        const next = fillListBooleans(child, current)
        if (next !== current) {
          copy ??= { ...value }
          Object.defineProperty(copy, child.id, { value: next, enumerable: true, writable: true, configurable: true })
        }
      }
    }
    return copy ?? value
  }
  if (field.type === "list" && Array.isArray(value)) {
    const entries = value.map((item) => fillListBooleans(field.item!, item))
    return entries.some((item, index) => item !== value[index]) ? entries : value
  }
  return value
}
