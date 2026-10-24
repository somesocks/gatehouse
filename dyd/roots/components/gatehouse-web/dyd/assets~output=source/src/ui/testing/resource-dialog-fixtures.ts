import { vi } from "vitest"
import { parseRoute } from "../../route"

const base = "/app/wsp/workspace/ses/chat"

export const resources = [
  { noun: "note", plural: "notes", name: "Named note", content: "The note content.", field: "session-note-title", form: "session-note-form" },
  { noun: "task", plural: "tasks", name: "Named task", content: "The task description.", field: "task-title", form: "task-form" },
  { noun: "secret", plural: "secrets", name: "Named secret", content: "Secret values are write-only.", field: "session-secret-description", form: "session-secret-form" },
] as const

export type ResourceCase = typeof resources[number]
export const collectionPath = (resource: ResourceCase) => `${base}/${resource.plural}`
export const readerPath = (resource: ResourceCase) => `${collectionPath(resource)}/${resource.noun}`
export const resourceRoute = (path: string) => parseRoute(new URL(path, "http://localhost"))

export function createResourceAPI(
  resource: ResourceCase,
  onDelete?: () => Promise<Response>,
  onWrite?: () => Promise<Response>,
) {
  const author = { principal: { name: "Author" }, name: "Author" }
  let data = {
    id: resource.noun as string,
    title: resource.name as string,
    description: resource.noun === "secret" ? resource.name : resource.noun === "task" ? resource.content : "Note summary",
    body: "The note content.",
    author,
    creator: author,
    updater: author,
    created_at: "2026-01-01T00:00:00.000Z",
    updated_at: "2026-01-01T00:00:00.000Z",
    sensitive: false,
    revision: 1,
    status: "draft",
  }
  const request = vi.fn(async (input: string, options?: RequestInit) => {
    const url = new URL(input, "http://localhost")
    if (url.pathname.endsWith("/sessions/chat"))
      return Response.json({ id: "chat", name: "Chat", created_at: data.created_at })
    if (!url.pathname.includes(`/sessions/chat/${resource.plural}`))
      throw new Error(`Unexpected request: ${input}`)
    if (options?.method === "DELETE")
      return onDelete ? onDelete() : new Response(null, { status: 204 })
    if (options?.method === "POST" || options?.method === "PATCH") {
      if (onWrite) return onWrite()
      data = { ...data, ...JSON.parse(String(options.body)), id: options.method === "POST" ? `created-${resource.noun}` : data.id }
      return Response.json(data)
    }
    if (url.pathname.endsWith(`/${resource.plural}`))
      return Response.json([data], { headers: url.searchParams.has("cursor") ? {} : { "X-Next-Cursor": "page-2" } })
    return Response.json(data)
  })
  return { request, get data() { return data } }
}
