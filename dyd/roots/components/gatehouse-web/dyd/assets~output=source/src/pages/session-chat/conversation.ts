import type { ChatBranch } from "../../app/chat"

export function emptyBranch(): ChatBranch {
  return { entries: [], agent_requests: [], interactions: [] }
}

// Pages are current reads, not a durable cross-page snapshot. Deduplicate by
// actual event ID, keeping the most recently fetched projection of an event.
export function mergeBranchPages(pages: ChatBranch[]): ChatBranch {
  const last = pages.at(-1) ?? emptyBranch()
  return {
    ...last,
    entries: [
      ...new Map(
        pages
          .flatMap((page) => page.entries)
          .map((entry) => [entry.event.ref.id, entry]),
      ).values(),
    ],
    agent_requests: [
      ...new Map(
        pages
          .flatMap((page) => page.agent_requests)
          .map((request) => [request.request.id, request]),
      ).values(),
    ],
    interactions: [
      ...new Map(
        pages
          .flatMap((page) => page.interactions)
          .map((interaction) => [interaction.request.ref.id, interaction]),
      ).values(),
    ],
  }
}

export function branchEventIDs(branch: ChatBranch): string[] {
  return [
    ...branch.entries.flatMap((entry) => [
      entry.event.ref.id,
      ...entry.previews.map((event) => event.ref.id),
    ]),
    ...branch.agent_requests.flatMap((request) => [
      request.request.id,
      ...(request.outcome ? [request.outcome.ref.id] : []),
      ...(request.outcome_previews?.map((event) => event.ref.id) ?? []),
    ]),
    ...branch.interactions.flatMap((interaction) => [
      interaction.request.ref.id,
      ...(interaction.response ? [interaction.response.ref.id] : []),
    ]),
  ]
}
