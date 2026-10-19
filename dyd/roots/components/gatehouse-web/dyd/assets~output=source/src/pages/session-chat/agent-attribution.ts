import type { TranscriptAgent } from "../../api-model"

export function agentLabel(agent: TranscriptAgent | undefined): string {
  return agent?.label ?? agent?.alias ?? agent?.ref.id ?? "Agent"
}
