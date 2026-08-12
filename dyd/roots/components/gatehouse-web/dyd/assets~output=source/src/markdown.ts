import DOMPurify from "dompurify"
import { marked } from "marked"

export function renderMarkdown(source: string) {
  return DOMPurify.sanitize(marked.parse(source) as string, {
    USE_PROFILES: { html: true },
  })
}
