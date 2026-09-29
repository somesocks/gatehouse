import { tick } from "svelte"
import {
  cancelChatReply,
  fetchChatAgents,
  fetchChatEvents,
  finishChatFileUpload,
  respondToChatApproval,
  sendChatMessage,
  startChatFileUpload,
  uploadChatFile,
  type ChatAgent,
  type ChatComposerFile,
  type ChatEvent,
  type ChatEventTree,
} from "../../app/chat"
import { mentionedAgentIDs } from "./chat-mentions"

type ChatControllerOptions = {
  isNearBottom: () => boolean
  followLatest: (behavior?: ScrollBehavior) => Promise<void>
  focusComposer: () => void
}

export type ChatDelivery =
  | { mode: "group" }
  | { mode: "direct"; agentID?: string }

export function createChatController({
  isNearBottom,
  followLatest,
  focusComposer,
}: ChatControllerOptions) {
  const state = $state({
    events: [] as ChatEventTree[],
    status: "checking" as "checking" | "ready" | "unavailable",
    agents: [] as ChatAgent[],
    messageText: "",
    composerFiles: [] as ChatComposerFile[],
    messageError: "",
    sendingMessage: false,
    awaitingReplyFor: [] as string[],
    cancellingReplyFor: new Set<string>(),
    submittingApprovals: new Set<string>(),
    approvalErrors: new Map<string, string>(),
    expandedActivity: new Set<string>(),
    showJumpToLatest: false,
    activityTimestamp: Date.now(),
    authenticationRequired: false,
  })
  let context: {
    workspaceID: string
    sessionID: string
    signal: AbortSignal
  } | null = null
  let generation = 0
  let followingLatest = false
  const current = (value: number) => value === generation
  const validContext = (
    value: number,
    workspaceID: string,
    sessionID: string,
  ) =>
    current(value) &&
    context?.workspaceID === workspaceID &&
    context?.sessionID === sessionID

  async function refreshEvents(
    value: number,
    showLoading = false,
  ): Promise<boolean> {
    if (context === null) return false
    const { workspaceID, sessionID, signal } = context
    if (showLoading) state.status = "checking"
    try {
      const response = await fetchChatEvents(workspaceID, sessionID, signal)
      if (!validContext(value, workspaceID, sessionID)) return false
      if (response.status === 401) {
        state.authenticationRequired = true
        return false
      }
      if (!response.ok) throw new Error("session events could not be loaded")
      const loaded = (await response.json()) as ChatEventTree[]
      if (!validContext(value, workspaceID, sessionID)) return false
      const knownEvents = new Set(state.events.flatMap(eventTreeIDs))
      const hasNewEvents = loaded.some((tree) =>
        eventTreeIDs(tree).some((id) => !knownEvents.has(id)),
      )
      const shouldFollow = showLoading || isNearBottom()
      state.events = loaded
      state.status = "ready"
      if (showLoading || hasNewEvents) {
        if (shouldFollow) void followLatest(showLoading ? "instant" : "smooth")
        else state.showJumpToLatest = true
      }
      const finishedReplies = new Set(
        loaded
          .filter(
            (tree) =>
              finalReplies(tree).length > 0 ||
              agentFailures(tree).length > 0 ||
              hasThinkingFailure(tree) ||
              hasCancellationSuccess(tree) ||
              hasCancellationFailure(tree),
          )
          .map((tree) => tree.event.ref.id),
      )
      if (
        state.awaitingReplyFor.some((eventID) => finishedReplies.has(eventID))
      )
        state.awaitingReplyFor = state.awaitingReplyFor.filter(
          (eventID) => !finishedReplies.has(eventID),
        )
      return true
    } catch {
      if (validContext(value, workspaceID, sessionID))
        state.status = "unavailable"
      return false
    }
  }

  async function refreshAgents(value: number): Promise<boolean> {
    if (context === null) return false
    const { workspaceID, sessionID, signal } = context
    try {
      const response = await fetchChatAgents(workspaceID, signal)
      if (!validContext(value, workspaceID, sessionID)) return false
      if (response.status === 401) {
        state.authenticationRequired = true
        return false
      }
      if (!response.ok) throw new Error("agents could not be refreshed")
      const agents = (await response.json()) as ChatAgent[]
      if (!validContext(value, workspaceID, sessionID)) return false
      state.agents = agents
      return true
    } catch {
      return false
    }
  }

  function start(
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
  ): void {
    stop()
    context = { workspaceID, sessionID, signal }
    const value = ++generation
    state.events = []
    state.status = "checking"
    state.agents = []
    state.messageText = ""
    state.composerFiles = []
    state.messageError = ""
    state.sendingMessage = false
    state.awaitingReplyFor = []
    state.cancellingReplyFor = new Set()
    state.submittingApprovals = new Set()
    state.approvalErrors = new Map()
    state.expandedActivity = new Set()
    state.showJumpToLatest = false
    state.activityTimestamp = Date.now()
    state.authenticationRequired = false
    followingLatest = false
    void Promise.all([refreshEvents(value, true), refreshAgents(value)])
  }

  function stop(): void {
    generation += 1
    context = null
    state.events = []
    state.agents = []
    state.messageText = ""
    state.composerFiles = []
    state.messageError = ""
    state.sendingMessage = false
    state.awaitingReplyFor = []
    state.cancellingReplyFor = new Set()
    state.submittingApprovals = new Set()
    state.approvalErrors = new Map()
    state.expandedActivity = new Set()
    state.showJumpToLatest = false
    followingLatest = false
  }

  async function sendMessage(
    delivery: ChatDelivery = { mode: "group" },
  ): Promise<void> {
    if (
      context === null ||
      state.sendingMessage ||
      (state.messageText.trim() === "" && state.composerFiles.length === 0)
    )
      return
    const { workspaceID, sessionID, signal } = context
    const value = generation
    const text = state.messageText
    if (delivery.mode === "direct" && delivery.agentID === undefined) {
      state.messageError =
        "The direct agent is unavailable. Choose another agent."
      return
    }
    const agents = deliveredAgentIDs(text, state.agents, delivery)
    state.messageError = ""
    state.sendingMessage = true
    try {
      const attachments = await Promise.all(
        state.composerFiles.map((entry) =>
          uploadComposerFile(entry, value, workspaceID, sessionID),
        ),
      )
      if (!validContext(value, workspaceID, sessionID)) return
      const response = await sendChatMessage(
        workspaceID,
        sessionID,
        {
          ...(text.trim() === "" ? {} : { text }),
          ...(agents.length === 0 ? {} : { agents }),
          ...(attachments.length === 0 ? {} : { attachments }),
        },
        signal,
      )
      if (!validContext(value, workspaceID, sessionID)) return
      if (response.status === 401) {
        state.authenticationRequired = true
        return
      }
      if (!response.ok) throw new Error("message could not be sent")
      const event = (await response.json()) as ChatEventTree<"message.text">["event"]
      if (event.kind !== "message.text") throw new Error("invalid message response")
      if (!validContext(value, workspaceID, sessionID)) return
      state.messageText = ""
      state.composerFiles = []
      await tick()
      state.events = [...state.events, { event, children: [] }]
      void followLatest()
      if (agents.length > 0)
        state.awaitingReplyFor = [...state.awaitingReplyFor, event.ref.id]
    } catch {
      if (current(value))
        state.messageError =
          "Your message or file upload could not be sent. Try again."
    } finally {
      if (current(value)) {
        state.sendingMessage = false
        await tick()
        focusComposer()
      }
    }
  }

  async function uploadComposerFile(
    entry: ChatComposerFile,
    value: number,
    workspaceID: string,
    sessionID: string,
  ): Promise<string> {
    if (entry.id !== undefined) return entry.id
    const signal = context?.signal
    updateComposerFile(entry.file, { status: "uploading", error: undefined })
    try {
      const created = await startChatFileUpload(
        workspaceID,
        sessionID,
        entry.file,
        signal,
      )
      if (!validContext(value, workspaceID, sessionID))
        throw new Error("stale upload")
      if (created.status === 401) {
        state.authenticationRequired = true
        throw new Error("authentication required")
      }
      if (!created.ok) throw new Error("create file failed")
      const upload = (await created.json()) as {
        file: { ref: { id: string } }
        upload_url: string
      }
      if (!validContext(value, workspaceID, sessionID))
        throw new Error("stale upload")
      const put = await uploadChatFile(upload.upload_url, entry.file, signal)
      if (!validContext(value, workspaceID, sessionID))
        throw new Error("stale upload")
      if (put.status === 401) {
        state.authenticationRequired = true
        throw new Error("authentication required")
      }
      if (!put.ok) throw new Error("upload file failed")
      const finished = await finishChatFileUpload(
        workspaceID,
        sessionID,
        upload.file.ref.id,
        signal,
      )
      if (!validContext(value, workspaceID, sessionID))
        throw new Error("stale upload")
      if (finished.status === 401) {
        state.authenticationRequired = true
        throw new Error("authentication required")
      }
      if (!finished.ok) throw new Error("finish file failed")
      updateComposerFile(entry.file, {
        id: upload.file.ref.id,
        status: "pending",
        error: undefined,
      })
      return upload.file.ref.id
    } catch (error) {
      if (validContext(value, workspaceID, sessionID))
        updateComposerFile(entry.file, {
          status: "failed",
          error: "Upload failed",
        })
      throw error
    }
  }

  async function cancelReply(tree: ChatEventTree): Promise<void> {
    if (context === null || state.cancellingReplyFor.has(tree.event.ref.id))
      return
    if (tree.event.kind !== "agent.request") return
    const { workspaceID, sessionID, signal } = context
    const value = generation
    state.cancellingReplyFor = new Set(state.cancellingReplyFor).add(
      tree.event.ref.id,
    )
    try {
      const response = await cancelChatReply(
        workspaceID,
        sessionID,
        tree.event.ref.id,
        signal,
      )
      if (!validContext(value, workspaceID, sessionID)) return
      if (response.status === 401) {
        state.authenticationRequired = true
        return
      }
      if (!response.ok) throw new Error("reply cancellation failed")
      await refreshEvents(value)
    } catch {
      if (current(value))
        state.messageError = "The reply could not be cancelled. Try again."
    } finally {
      if (current(value)) {
        const pending = new Set(state.cancellingReplyFor)
        pending.delete(tree.event.ref.id)
        state.cancellingReplyFor = pending
      }
    }
  }

  async function respondToApproval(
    approval: ChatEventTree,
    decision: "approved" | "rejected",
  ): Promise<void> {
    if (
      context === null ||
      state.submittingApprovals.has(approval.event.ref.id)
    )
      return
    const { workspaceID, sessionID, signal } = context
    const value = generation
    state.submittingApprovals = new Set(state.submittingApprovals).add(
      approval.event.ref.id,
    )
    const errors = new Map(state.approvalErrors)
    errors.delete(approval.event.ref.id)
    state.approvalErrors = errors
    try {
      const response = await respondToChatApproval(
        workspaceID,
        sessionID,
        approval.event.ref.id,
        decision,
        signal,
      )
      if (!validContext(value, workspaceID, sessionID)) return
      if (response.status === 401) {
        state.authenticationRequired = true
        return
      }
      if (response.status === 409)
        throw new Error("This approval has already been decided.")
      if (!response.ok)
        throw new Error(
          "The approval response could not be submitted. Try again.",
        )
      await refreshEvents(value)
    } catch (error) {
      if (current(value)) {
        const next = new Map(state.approvalErrors)
        next.set(
          approval.event.ref.id,
          error instanceof Error
            ? error.message
            : "The approval response could not be submitted. Try again.",
        )
        state.approvalErrors = next
      }
    } finally {
      if (current(value)) {
        const pending = new Set(state.submittingApprovals)
        pending.delete(approval.event.ref.id)
        state.submittingApprovals = pending
      }
    }
  }

  function updateComposerFile(
    file: File,
    update: Partial<ChatComposerFile>,
  ): void {
    state.composerFiles = state.composerFiles.map((entry) =>
      entry.file === file ? { ...entry, ...update } : entry,
    )
  }
  function selectComposerFiles(input: HTMLInputElement): void {
    const selected = Array.from(input.files ?? [])
    state.composerFiles = [
      ...state.composerFiles,
      ...selected.map((file) => ({ file, status: "pending" as const })),
    ]
    input.value = ""
  }
  function removeComposerFile(file: File): void {
    state.composerFiles = state.composerFiles.filter(
      (entry) => entry.file !== file,
    )
  }
  function toggleActivity(tree: ChatEventTree): void {
    const expanded = new Set(state.expandedActivity)
    expanded.has(tree.event.ref.id)
      ? expanded.delete(tree.event.ref.id)
      : expanded.add(tree.event.ref.id)
    state.expandedActivity = expanded
  }
  function trackScroll(): void {
    if (followingLatest) {
      if (isNearBottom()) followingLatest = false
      else return
    }
    state.showJumpToLatest = !isNearBottom()
  }
  async function jumpToLatest(): Promise<void> {
    followingLatest = true
    state.showJumpToLatest = false
    await followLatest()
    if (isNearBottom()) followingLatest = false
  }
  function updateActivityTimestamp(): void {
    state.activityTimestamp = Date.now()
  }

  return {
    state,
    start,
    stop,
    refreshEvents: () => refreshEvents(generation),
    refreshAgents: () => refreshAgents(generation),
    sendMessage,
    cancelReply,
    respondToApproval,
    selectComposerFiles,
    removeComposerFile,
    toggleActivity,
    trackScroll,
    jumpToLatest,
    updateActivityTimestamp,
  }
}

export function eventTreeIDs(tree: ChatEventTree): string[] {
  return [tree.event.ref.id, ...tree.children.flatMap(eventTreeIDs)]
}
export function deliveredAgentIDs(
  text: string,
  agents: ChatAgent[],
  delivery: ChatDelivery,
): string[] {
  const mentions = mentionedAgentIDs(text, agents)
  if (mentions.length > 0 || delivery.mode === "group") return mentions
  return delivery.agentID === undefined ? [] : [delivery.agentID]
}
function hasKind<K extends ChatEvent["kind"]>(
  tree: ChatEventTree,
  kind: K,
): tree is ChatEventTree<K> {
  return tree.event.kind === kind
}

export function finalReplies(
  tree: ChatEventTree,
): ChatEventTree<"agent.success">[] {
  return tree.children.flatMap((child) => [
    ...(hasKind(child, "agent.success") &&
    child.event.author_agent !== undefined
      ? [child]
      : []),
    ...finalReplies(child),
  ])
}
export function agentFailures(
  tree: ChatEventTree,
): ChatEventTree<"agent.failure">[] {
  return tree.children.filter((child) => hasKind(child, "agent.failure"))
}
export function agentRequests(
  tree: ChatEventTree,
): ChatEventTree<"agent.request">[] {
  return tree.children.filter((child) => hasKind(child, "agent.request"))
}
export function activityEvents(tree: ChatEventTree): ChatEventTree[] {
  return tree.children.flatMap((child) => [
    ...(child.event.kind !== "agent.request" &&
    child.event.kind !== "agent.success" &&
    child.event.kind !== "agent.failure"
      ? [child]
      : []),
    ...activityEvents(child),
  ])
}
export function renderedActivityEvents(
  tree: ChatEventTree,
): ChatEventTree<"tool.request" | "thinking.request">[] {
  return activityEvents(tree).filter(
    (
      activity,
    ): activity is ChatEventTree<"tool.request" | "thinking.request"> =>
      hasKind(activity, "tool.request") ||
      hasKind(activity, "thinking.request"),
  )
}
export function displayedActivityEvents(
  tree: ChatEventTree,
  expanded: Set<string>,
): ChatEventTree<"tool.request" | "thinking.request">[] {
  const activity = renderedActivityEvents(tree)
  return expanded.has(tree.event.ref.id) || activity.length <= 5
    ? activity
    : activity.slice(-5)
}
export function hasThinkingFailure(tree: ChatEventTree): boolean {
  return activityEvents(tree).some(
    (child) =>
      child.event.kind === "thinking.request" &&
      thinkingStatus(child) === "failed",
  )
}
export function cancellationRequest(
  tree: ChatEventTree,
): ChatEventTree<"cancel.request"> | undefined {
  for (const child of tree.children) {
    if (hasKind(child, "cancel.request")) return child
    const nested = cancellationRequest(child)
    if (nested !== undefined) return nested
  }
  return undefined
}
export function hasCancellationSuccess(tree: ChatEventTree): boolean {
  return (
    cancellationRequest(tree)?.children.some(
      (child) => child.event.kind === "cancel.success",
    ) ?? false
  )
}
export function hasCancellationFailure(tree: ChatEventTree): boolean {
  return (
    cancellationRequest(tree)?.children.some(
      (child) => child.event.kind === "cancel.failure",
    ) ?? false
  )
}
export function replyCanBeCancelled(tree: ChatEventTree): boolean {
  return (
    (tree.event.kind === "agent.request" || agentRequests(tree).length === 1) &&
    finalReplies(tree).length === 0 &&
    agentFailures(tree).length === 0 &&
    !hasThinkingFailure(tree) &&
    cancellationRequest(tree) === undefined
  )
}
export function activityStatus(
  tree: ChatEventTree,
  completedKind: string,
  failedKind: string,
): "working" | "succeeded" | "failed" {
  if (tree.children.some((child) => child.event.kind === failedKind))
    return "failed"
  return tree.children.some((child) => child.event.kind === completedKind)
    ? "succeeded"
    : "working"
}
export function toolStatus(
  tree: ChatEventTree,
): "working" | "succeeded" | "failed" {
  return activityStatus(tree, "tool.success", "tool.failure")
}
export function thinkingStatus(
  tree: ChatEventTree,
): "working" | "succeeded" | "failed" {
  return activityStatus(tree, "thinking.success", "thinking.failure")
}
export function thinkingRateLimitDelayUntil(
  tree: ChatEventTree,
  now: number,
): string | undefined {
  for (let index = tree.children.length - 1; index >= 0; index -= 1) {
    const delay = tree.children[index].event
    if (
      delay.kind !== "thinking.update" ||
      delay.payload.reason !== "rate_limit" ||
      typeof delay.payload.until !== "string"
    )
      continue
    const until = new Date(delay.payload.until).getTime()
    if (Number.isFinite(until) && until > now) return delay.payload.until
  }
  return undefined
}
export function approvalRequests(
  tree: ChatEventTree,
): ChatEventTree<"approval.request">[] {
  return tree.children.filter((child) => hasKind(child, "approval.request"))
}
export function approvalResponse(
  tree: ChatEventTree,
): ChatEventTree<"approval.success" | "approval.failure"> | undefined {
  return tree.children.find(
    (child): child is ChatEventTree<"approval.success" | "approval.failure"> =>
      child.event.kind === "approval.success" ||
      child.event.kind === "approval.failure",
  )
}

export function inputRequests(
  tree: ChatEventTree,
): ChatEventTree<"input.request">[] {
  return tree.children.filter((child) => hasKind(child, "input.request"))
}

export function inputResponse(
  tree: ChatEventTree,
): ChatEventTree<"input.success" | "input.failure"> | undefined {
  return tree.children.find(
    (child): child is ChatEventTree<"input.success" | "input.failure"> =>
      child.event.kind === "input.success" ||
      child.event.kind === "input.failure",
  )
}

export function elapsedDuration(
  startedAt: string,
  completedAt: string | number,
): string {
  const elapsed =
    new Date(completedAt).getTime() - new Date(startedAt).getTime()
  if (!Number.isFinite(elapsed) || elapsed < 0) return ""
  if (elapsed < 100) return "<0.1s"
  if (elapsed >= 60_000) {
    const seconds = Math.floor(elapsed / 1000)
    return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  }
  return `${(elapsed / 1000).toFixed(1)}s`
}
export function activityDuration(
  tree: ChatEventTree,
  completedKind: string,
  failedKind: string,
): string {
  const completed = tree.children.find(
    (child) =>
      child.event.kind === completedKind || child.event.kind === failedKind,
  )
  return completed === undefined
    ? ""
    : elapsedDuration(tree.event.created_at, completed.event.created_at)
}
