export function userAuthorLabel(author: {
  principal?: { name?: string }
  agent?: { label?: string }
  gateway?: string
}): string {
  if (author.principal) return author.principal.name ?? "User"
  if (author.agent) return author.agent.label ?? "Agent"
  if (author.gateway) return "Gateway"
  return "Unknown"
}
