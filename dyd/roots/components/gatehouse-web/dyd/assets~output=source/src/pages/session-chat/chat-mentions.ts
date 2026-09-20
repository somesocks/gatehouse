import type { ChatAgent } from "../../app/chat"

function aliasCharacter(value: string): boolean {
  return /[a-z0-9_/-]/.test(value)
}

export function mentionedAgentIDs(text: string, agents: ChatAgent[]): string[] {
  const agentsByAlias = new Map(agents.map((agent) => [agent.alias, agent.id]))
  const ids: string[] = []
  const seen = new Set<string>()
  for (let index = 0; index < text.length; index += 1) {
    if (
      text[index] !== "@" ||
      (index > 0 && aliasCharacter(text[index - 1]))
    )
      continue
    let end = index + 1
    while (end < text.length && aliasCharacter(text[end])) end += 1
    const id = agentsByAlias.get(text.slice(index + 1, end))
    if (id !== undefined && !seen.has(id)) {
      seen.add(id)
      ids.push(id)
    }
    index = end - 1
  }
  return ids
}
