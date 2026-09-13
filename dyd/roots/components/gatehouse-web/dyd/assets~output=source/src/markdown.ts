import DOMPurify from "dompurify"
import { marked, Renderer } from "marked"
import markedKatex from "marked-katex-extension"

const renderer = new Renderer()
const renderCode = renderer.code.bind(renderer)
renderer.code = (token) =>
  `<div class="markdown-code-block"><button class="markdown-code-copy" type="button" aria-label="Copy code" title="Copy code"><svg class="lucide lucide-copy" xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg></button>${renderCode(token)}</div>`
marked.use({ renderer })
marked.use(
  markedKatex({
    throwOnError: false,
    output: "html",
    maxSize: 100,
    errorColor: "#ffb4b4",
  }),
)

export function renderMarkdown(source: string) {
  return DOMPurify.sanitize(marked.parse(source) as string, {
    USE_PROFILES: { html: true, svg: true },
  })
}
