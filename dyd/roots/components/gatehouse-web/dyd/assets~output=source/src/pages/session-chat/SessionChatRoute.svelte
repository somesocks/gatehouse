<script lang="ts">
  import { onMount, tick, untrack } from "svelte"
  import {
    Bot,
    CircleCheck,
    CircleX,
    Copy,
    Paperclip,
    Send,
    ShieldCheck,
    ShieldQuestionMark,
    ShieldX,
    X,
  } from "@lucide/svelte"
  import { Command, Dialog } from "bits-ui"
  import {
    chatFileDownloadPath,
    fetchChatSession,
    type ChatAgent,
    type ChatEventTree,
  } from "../../app/chat"
  import { useRuntime } from "../../app/runtime.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import SessionNavigation from "../../components/SessionNavigation.svelte"
  import StatusPage from "../../components/StatusPage.svelte"
  import * as SidebarPage from "../../components/sidebar-page"
  import WorkspaceNavigation from "../../components/WorkspaceNavigation.svelte"
  import { renderMarkdown } from "../../markdown"
  import type { Route } from "../../route"
  import {
    activityDuration,
    activityEvents,
    agentRequests,
    approvalRequests,
    approvalResponse,
    cancellationRequest,
    type ChatDelivery,
    createChatController,
    displayedActivityEvents,
    elapsedDuration,
    finalReplies,
    hasCancellationSuccess,
    renderedActivityEvents,
    replyCanBeCancelled,
    thinkingRateLimitDelayUntil,
    thinkingStatus,
    toolStatus,
  } from "./chat-controller.svelte"

  type SessionRoute = Extract<Route, { kind: "session-chat" }>
  type SessionTarget = Pick<SessionRoute, "workspaceID" | "sessionID">
  type Session = {
    id: string
    created_at: string
    name?: string
    project?: { id: string; name?: string }
  }
  type Status = "checking" | "ready" | "unavailable"

  const runtime = useRuntime()
  const { access, activity, auth } = runtime
  let session = $state<Session | null>(null)
  let sessionStatus = $state<Status>("checking")
  let messageInputElement = $state<HTMLTextAreaElement | undefined>()
  let fileInputElement = $state<HTMLInputElement | undefined>()
  let mentionSearchInputElement = $state<HTMLInputElement | undefined>()
  let deliverySearchInputElement = $state<HTMLInputElement | undefined>()
  let mentionOpen = $state(false)
  let mentionQuery = $state("")
  let deliveryQuery = $state("")
  let mentionStart = -1
  let pendingMentionStart: number | undefined
  let deliveryOpen = $state(false)
  let generation = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as SessionRoute)
  const sessionKey = $derived(
    `${currentRoute.workspaceID}\u0000${currentRoute.sessionID}`,
  )
  const workspace = $derived(
    access.state.workspaces.find(
      (candidate) => candidate.id === currentRoute.workspaceID,
    ) ?? null,
  )
  const workspacePath = (workspaceID: string) =>
    `/app/wsp/${encodeURIComponent(workspaceID)}`
  const sessionsPath = (workspaceID: string) =>
    `${workspacePath(workspaceID)}/ses`
  const sessionPath = (workspaceID: string, sessionID: string) =>
    `${sessionsPath(workspaceID)}/${encodeURIComponent(sessionID)}`
  const projectPath = (workspaceID: string, projectID: string) =>
    `${workspacePath(workspaceID)}/prj/${encodeURIComponent(projectID)}`
  const isCurrent = (
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
  ) =>
    value === generation &&
    !signal.aborted &&
    currentRoute.workspaceID === workspaceID &&
    currentRoute.sessionID === sessionID
  function updateSessionName(name: string): void {
    if (session !== null) session = { ...session, name }
  }

  function isNearBottom(): boolean {
    const scrollingElement = document.scrollingElement
    return (
      scrollingElement === null ||
      scrollingElement.scrollHeight -
        scrollingElement.scrollTop -
        scrollingElement.clientHeight <
        64
    )
  }
  async function followLatest(
    behavior: ScrollBehavior = "smooth",
  ): Promise<void> {
    await tick()
    const scrollingElement = document.scrollingElement
    scrollingElement?.scrollTo({
      top: scrollingElement.scrollHeight,
      behavior,
    })
  }
  const controller = untrack(() =>
    createChatController({
      isNearBottom,
      followLatest,
      focusComposer: () => messageInputElement?.focus(),
    }),
  )
  const filteredMentionAgents = $derived(
    controller.state.agents.filter((agent) => {
      const query = mentionQuery.trim().toLowerCase()
      return (
        query === "" ||
        agent.alias.includes(query) ||
        agent.label?.toLowerCase().includes(query) === true
      )
    }),
  )
  const filteredDeliveryAgents = $derived(
    controller.state.agents.filter((agent) => {
      const query = deliveryQuery.trim().toLowerCase()
      return (
        query === "" ||
        "direct".includes(query) ||
        agent.alias.includes(query) ||
        agent.label?.toLowerCase().includes(query) === true
      )
    }),
  )
  const groupDeliveryVisible = $derived(
    "group chat".includes(deliveryQuery.trim().toLowerCase()),
  )
  let directAgentID = $state<string | null>(currentRoute.agent)
  let deliveryMode = $state<ChatDelivery["mode"]>(currentRoute.mode)
  const directAgent = $derived.by(() => {
    if (deliveryMode !== "direct") return undefined
    return controller.state.agents.find((agent) =>
      directAgentID === null ? agent.default : agent.id === directAgentID,
    )
  })
  const delivery = $derived<ChatDelivery>(
    deliveryMode === "direct"
      ? { mode: "direct", agentID: directAgent?.id }
      : { mode: "group" },
  )
  const deliveryTitle = $derived(
    deliveryMode === "group"
      ? "Group chat"
      : directAgent === undefined
        ? "Direct chat"
        : `Direct chat with @${directAgent.alias}`,
  )

  $effect(() => {
    const [workspaceID, sessionID] = sessionKey.split("\u0000")
    return activate({ workspaceID, sessionID })
  })
  $effect(() => {
    directAgentID = currentRoute.agent
    deliveryMode = currentRoute.mode
  })
  $effect(() => {
    if (controller.state.authenticationRequired) runtime.requireLogin()
  })
  $effect(() => {
    const track = () => controller.trackScroll()
    window.addEventListener("scroll", track, { passive: true })
    return () => window.removeEventListener("scroll", track)
  })

  onMount(() => {
    const copyCodeBlock = (event: MouseEvent) => {
      if (!(event.target instanceof Element)) return
      const button = event.target.closest<HTMLButtonElement>(
        ".code-copy",
      )
      const code = button?.parentElement?.querySelector("pre > code")
      if (code !== null && code !== undefined)
        void copyMarkdown(code.textContent ?? "")
    }
    document.addEventListener("click", copyCodeBlock)
    const clock = window.setInterval(
      () => controller.updateActivityTimestamp(),
      1000,
    )
    return () => {
      document.removeEventListener("click", copyCodeBlock)
      window.clearInterval(clock)
    }
  })

  function activate(route: SessionTarget): () => void {
    const value = ++generation
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    controller.stop()
    session = null
    sessionStatus = "checking"
    if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "ready"
    )
      void loadRoute(route, value, abortController.signal)
    else if (
      auth.state.status === "authenticated" &&
      access.state.workspaceStatus === "checking"
    )
      void runtime.refresh()
    return () => {
      if (value === generation) {
        abortController?.abort()
        unsubscribe?.()
        unsubscribe = undefined
        controller.stop()
      }
    }
  }

  async function loadRoute(
    route: SessionTarget,
    value: number,
    signal: AbortSignal,
  ): Promise<void> {
    const { workspaceID, sessionID } = route
    if (
      !access.state.workspaces.some((candidate) => candidate.id === workspaceID)
    ) {
      if (isCurrent(value, workspaceID, sessionID, signal))
        runtime.navigate(
          access.state.workspaces.length === 0
            ? "/app/no-access"
            : workspacePath(access.state.workspaces[0].id),
          true,
        )
      return
    }
    if (!(await loadSession(value, workspaceID, sessionID, signal))) return
    if (!isCurrent(value, workspaceID, sessionID, signal)) return
    controller.start(workspaceID, sessionID, signal)
    subscribe(route, value, signal)
  }

  async function loadSession(
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    if (showLoading && isCurrent(value, workspaceID, sessionID, signal))
      sessionStatus = "checking"
    try {
      const response = await fetchChatSession(workspaceID, sessionID, signal)
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      if (response.status === 401) {
        runtime.requireLogin()
        return false
      }
      if (response.status === 404) {
        runtime.navigate(sessionsPath(workspaceID), true)
        return false
      }
      if (!response.ok) throw new Error("session unavailable")
      const loaded = (await response.json()) as Session
      if (!isCurrent(value, workspaceID, sessionID, signal)) return false
      session = loaded
      sessionStatus = "ready"
      return true
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        sessionStatus = "unavailable"
      return false
    }
  }

  function subscribe(
    route: SessionTarget,
    value: number,
    routeSignal: AbortSignal,
  ): void {
    const { workspaceID, sessionID } = route
    unsubscribe = activity.subscribe(
      [
        {
          name: "session-chat",
          topic: `${workspaceID}/${sessionID}`,
          events: [
            "session.*",
            "session_event.*",
            "session_file.*",
            "session_note.*",
            "session_secret.*",
          ],
        },
        {
          name: "session-chat-agent",
          topic: workspaceID,
          events: ["workspace_agent.*"],
        },
      ],
      async ({ names, signal }) => {
        if (
          !isCurrent(value, workspaceID, sessionID, routeSignal) ||
          signal.aborted
        )
          return
        controller.updateActivityTimestamp()
        const refreshed = await Promise.all([
          ...(names.has("session-chat")
            ? [
                controller.refreshEvents(),
                loadSession(value, workspaceID, sessionID, routeSignal, false),
              ]
            : []),
          ...(names.has("session-chat-agent")
            ? [controller.refreshAgents()]
            : []),
        ])
        if (
          !refreshed.every(Boolean) ||
          signal.aborted ||
          !isCurrent(value, workspaceID, sessionID, routeSignal)
        )
          throw new Error("session chat refresh failed")
      },
    )
    void activity.poll()
  }

  async function copyMarkdown(text: string): Promise<void> {
    await navigator.clipboard.writeText(text)
  }
  function agentLabel(id: string): string {
    return controller.state.agents.find((agent) => agent.id === id)?.label ?? id
  }
  function replyDuration(tree: ChatEventTree): string {
    const replies = finalReplies(tree)
    return replies.length === 0
      ? ""
      : elapsedDuration(
          tree.event.created_at,
          replies[replies.length - 1].event.created_at,
        )
  }
  function workingReplyDuration(tree: ChatEventTree): string {
    return elapsedDuration(
      tree.event.created_at,
      controller.state.activityTimestamp,
    )
  }
  function thinkingSummary(tree: ChatEventTree): string {
    const status = thinkingStatus(tree)
    if (status === "succeeded") return "Thought"
    if (status === "failed") return "Thinking failed after"
    const until = thinkingRateLimitDelayUntil(
      tree,
      controller.state.activityTimestamp,
    )
    return until === undefined
      ? "Thinking"
      : `Waiting for rate limit until ${new Date(until).toLocaleTimeString()}`
  }
  function activityAgentLabel(tree: ChatEventTree): string {
    const request = agentRequests(tree)[0]
    if (request?.event.payload.agent !== undefined)
      return agentLabel(request.event.payload.agent)
    if (tree.event.payload.agent !== undefined)
      return agentLabel(tree.event.payload.agent)
    const activity = activityEvents(tree).find(
      (child) => child.event.author_agent !== undefined,
    )
    if (activity?.event.author_agent !== undefined)
      return agentLabel(activity.event.author_agent.id)
    const reply = finalReplies(tree)[0]
    return reply?.event.author_agent === undefined
      ? "Agent"
      : agentLabel(reply.event.author_agent.id)
  }
  function approvalDescription(
    approval: ChatEventTree,
    task: ChatEventTree,
  ): string {
    return (
      approval.event.payload.description ??
      task.event.payload.reason ??
      `Run ${task.event.payload.name ?? "tool"}`
    )
  }
  function handleMentionKeydown(event: KeyboardEvent): void {
    if (
      event.key !== "@" ||
      event.isComposing ||
      messageInputElement === undefined
    )
      return
    pendingMentionStart = messageInputElement.selectionStart
  }
  function openMentionAfterInput(): void {
    if (pendingMentionStart === undefined) return
    mentionStart = pendingMentionStart
    pendingMentionStart = undefined
    mentionQuery = ""
    mentionOpen = true
  }
  function focusComposer(cursor?: number): void {
    void tick().then(() => {
      messageInputElement?.focus({ preventScroll: true })
      if (cursor !== undefined)
        messageInputElement?.setSelectionRange(cursor, cursor)
    })
  }
  function selectMention(agent: ChatAgent): void {
    if (mentionStart < 0) return
    const start = mentionStart
    mentionStart = -1
    const alias = `@${agent.alias}`
    const text = controller.state.messageText
    controller.state.messageText =
      text.slice(0, start) + alias + text.slice(start + 1)
    mentionOpen = false
    focusComposer(start + alias.length)
  }
  function setGroupDelivery(): void {
    deliveryMode = "group"
    directAgentID = null
    deliveryOpen = false
    window.history.replaceState(
      null,
      "",
      sessionPath(currentRoute.workspaceID, currentRoute.sessionID),
    )
  }
  function setDirectDelivery(agentID?: string): void {
    const parameters = new URLSearchParams({ mode: "direct" })
    if (agentID !== undefined) parameters.set("agent", agentID)
    deliveryMode = "direct"
    directAgentID = agentID ?? null
    deliveryOpen = false
    window.history.replaceState(
      null,
      "",
      `${sessionPath(currentRoute.workspaceID, currentRoute.sessionID)}?${parameters}`,
    )
  }
  function openDelivery(): void {
    deliveryQuery = ""
    deliveryOpen = true
  }
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <StatusPage eyebrow="Gatehouse" busy live="polite">
    {#snippet children()}
      <p>
        {auth.state.status === "checking"
          ? "Checking your session."
          : "Loading your workspaces."}
      </p>
    {/snippet}
  </StatusPage>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage eyebrow="Gatehouse" title="Connection unavailable" description="Gatehouse could not load your account.">
    {#snippet children()}<div><button class="primary" type="button" onclick={() => void runtime.refresh()}>Try again</button></div>{/snippet}
  </StatusPage>
{:else if auth.state.status !== "authenticated"}
  <StatusPage eyebrow="Gatehouse" title="Sign in required">
    {#snippet children()}<div><button class="primary" type="button" onclick={() => runtime.requireLogin()}>Sign in</button></div>{/snippet}
  </StatusPage>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <StatusPage eyebrow="Gatehouse" title="No workspace access" description="Ask an administrator to add you to a workspace group.">
    {#snippet children()}<div><button class="primary" type="button" onclick={() => runtime.navigate("/app/no-access", true)}>Continue</button></div>{/snippet}
  </StatusPage>
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar
      ><WorkspaceNavigation {workspace} active="chats" /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header placement="floating">
        <SidebarPage.Toggle />
        <nav aria-label="Breadcrumb" data-page-breadcrumb>
          <ol>
            <li><RouterLink href={workspacePath(workspace.id)}>{workspace.name ?? workspace.id}</RouterLink></li>
            {#if session !== null}
              {#if session.project !== undefined}
                <li><RouterLink href={projectPath(workspace.id, session.project.id)}>{session.project.name ?? "New Project"}</RouterLink></li>
              {/if}
              <li aria-current="page">{session.name ?? "New Chat"}</li>
            {/if}
          </ol>
        </nav>
        {#if session !== null}<SessionNavigation
            workspaceID={workspace.id}
            sessionID={session.id}
            active="chat"
            name={session.name}
            onRenamed={updateSessionName}
          />{/if}
      </SidebarPage.Header>
      <SidebarPage.Body>
        {#if sessionStatus === "unavailable"}<p class="muted">
            This chat could not be loaded.
          </p>
          <button
            class="primary"
            type="button"
            onclick={() =>
              void loadRoute(currentRoute, generation, abortController!.signal)}
            >Try again</button
          >
        {:else if sessionStatus === "checking" || controller.state.status === "checking"}<div
            class="conversation-feed"
            data-loading
            aria-busy="true"
            aria-live="polite"
          >
            <p class="conversation-status"><span class="spinner" role="status"
                ><span class="visually-hidden">Loading</span></span
              > Loading chat...</p
            >
          </div>
        {:else if session !== null}
          <div class="conversation-feed" aria-live="polite">
            {#if controller.state.status === "unavailable"}<p
                class="conversation-status"
              >
                This chat could not be loaded.
              </p>{:else if controller.state.events.length === 0}<p
                class="conversation-status"
              >
                Send the first message to begin.
              </p>{:else}{#each controller.state.events as tree (tree.event.ref.id)}{#if tree.event.kind === "message.text" && (tree.event.payload.text !== undefined || (tree.event.payload.attachments !== undefined && tree.event.payload.attachments.length > 0))}<article
                    class="conversation-message card"
                    data-align="end"
                  >
                    <header>
                      {tree.event.author_principal?.name ?? "User"}
                    </header>
                    {#if tree.event.payload.text !== undefined}<button
                        class="icon inline small"
                        type="button"
                        aria-label="Copy message Markdown"
                        title="Copy Markdown"
                        onclick={() =>
                          void copyMarkdown(tree.event.payload.text)}
                        ><Copy size={16} strokeWidth={2} /></button
                      >
                      <div class="prose">
                        {@html renderMarkdown(tree.event.payload.text)}
                      </div>{/if}{#if tree.event.payload.attachments !== undefined && tree.event.payload.attachments.length > 0}<div
                        class="attachment-list"
                        aria-label="Attached files"
                      >
                        {#each tree.event.payload.attachments as file (file.id)}<a
                            class="attachment-chip badge"
                            href={chatFileDownloadPath(
                              workspace.id,
                              session.id,
                              file.id,
                            )}
                            target="_blank"
                            rel="noopener noreferrer"
                            download={file.name}
                            title={file.fingerprint}
                            ><Paperclip
                              size={14}
                              strokeWidth={2}
                              aria-hidden="true"
                            /><span>{file.name}</span><small
                              >{file.size} bytes{file.media_type === undefined
                                ? ""
                                : ` · ${file.media_type}`}</small
                            ></a
                          >{/each}
                      </div>{/if}
                  </article>
                  {#each agentRequests(tree) as request (request.event.ref.id)}<section
                      class="event-log"
                    >
                      <header>
                        {hasCancellationSuccess(request)
                          ? "Cancelled"
                          : cancellationRequest(request) !== undefined
                            ? "Cancellation requested"
                            : finalReplies(request).length === 0
                              ? `${activityAgentLabel(request)} is working`
                              : activityAgentLabel(
                                  request,
                                )}{#if finalReplies(request).length > 0 && replyDuration(request) !== ""}<small>{replyDuration(request)}</small>{/if}{#if replyCanBeCancelled(request) && workingReplyDuration(request) !== ""}<small>{workingReplyDuration(request)}</small>{/if}{#if replyCanBeCancelled(request)}<button
                            class="inline"
                            type="button"
                            disabled={controller.state.cancellingReplyFor.has(
                              request.event.ref.id,
                            )}
                            onclick={() => void controller.cancelReply(request)}
                            >{controller.state.cancellingReplyFor.has(
                              request.event.ref.id,
                            )
                              ? "Cancelling..."
                              : "Cancel"}</button
                          >{/if}
                      </header>
                      {#if renderedActivityEvents(request).length > 0}<div
                          class="event-list"
                        >
                          {#if renderedActivityEvents(request).length > 5 && !controller.state.expandedActivity.has(request.event.ref.id)}<p
                              class="event-more"
                            >
                              <span
                                >({renderedActivityEvents(request).length - 5} more)</span
                               ><button
                                class="primary inline"
                                type="button"
                              onclick={() => controller.toggleActivity(request)}
                                >Show all</button
                              >
                            </p>{/if}{#each displayedActivityEvents(request, controller.state.expandedActivity) as event (event.event.ref.id)}{#if event.event.kind === "tool.request"}<p
                                class="event-summary"
                                data-state={toolStatus(event)}
                                title={event.event.payload.name ?? "tool"}
                              >
                                {#if toolStatus(event) === "working"}<span
                                    class="spinner"
                                    aria-hidden="true"

                                  ></span>{:else if toolStatus(event) === "succeeded"}<CircleCheck
                                    size={14}
                                    strokeWidth={2}
                                    aria-hidden="true"
                                  />{:else}<CircleX
                                    size={14}
                                    strokeWidth={2}
                                    aria-hidden="true"
                                  />{/if}Task: {event.event.payload.reason ??
                                  `Running ${event.event.payload.name ?? "tool"}`}{#if activityDuration(event, "tool.success", "tool.failure") !== ""}<small>{activityDuration(
                                      event,
                                      "tool.success",
                                      "tool.failure",
                                    )}</small>{/if}
                              </p>
                              {#each approvalRequests(event) as approval (approval.event.ref.id)}{@const response =
                                  approvalResponse(approval)}
                                <section class="event-request" data-resolved={response !== undefined || undefined}>
                                  {#if response === undefined}<ShieldQuestionMark
                                      size={15}
                                      strokeWidth={2}
                                      aria-hidden="true"
                                    /><strong>Action approval required:</strong
                                    ><span>{approvalDescription(
                                        approval,
                                        event,
                                      )}</span
                                    ><span data-actions
                                      ><button
                                        class="primary small"
                                        type="button"
                                        disabled={controller.state.submittingApprovals.has(
                                          approval.event.ref.id,
                                        )}
                                        onclick={() =>
                                          void controller.respondToApproval(
                                            approval,
                                            "approved",
                                          )}
                                        >{controller.state.submittingApprovals.has(
                                          approval.event.ref.id,
                                        )
                                          ? "Submitting..."
                                          : "Approve"}</button
                                      ><button
                                        class="secondary small"
                                        type="button"
                                        disabled={controller.state.submittingApprovals.has(
                                          approval.event.ref.id,
                                        )}
                                        onclick={() =>
                                          void controller.respondToApproval(
                                            approval,
                                            "rejected",
                                          )}>Reject</button
                                      ></span
                                    >{:else if response.event.kind === "approval.approved"}<ShieldCheck
                                      size={15}
                                      strokeWidth={2}
                                      aria-hidden="true"
                                    /><strong>Action approved:</strong
                                    ><span>{approvalDescription(
                                        approval,
                                        event,
                                      )}</span
                                    >{:else}<ShieldX
                                      size={15}
                                      strokeWidth={2}
                                      aria-hidden="true"
                                    /><strong>Action rejected:</strong
                                    ><span>{approvalDescription(
                                        approval,
                                        event,
                                      )}</span
                                    >{/if}
                                </section>
                                {#if response === undefined && (controller.state.approvalErrors.get(approval.event.ref.id) ?? "") !== ""}<p
                                    class="event-error"
                                    role="alert"
                                  >
                                    {controller.state.approvalErrors.get(
                                      approval.event.ref.id,
                                    )}
                              </p>{/if}{/each}{:else if event.event.kind === "thinking.started"}<p
                                class="event-summary"
                                data-state={thinkingStatus(event)}
                              >
                                {#if thinkingStatus(event) === "working"}<span
                                    class="spinner"
                                    aria-hidden="true"

                                  ></span>{:else if thinkingStatus(event) === "succeeded"}<CircleCheck
                                    size={14}
                                    strokeWidth={2}
                                    aria-hidden="true"
                                  />{:else}<CircleX
                                    size={14}
                                    strokeWidth={2}
                                    aria-hidden="true"
                                  />{/if}{thinkingSummary(event)}{#if activityDuration(event, "thinking.completed", "thinking.failed") !== ""}<small>{activityDuration(
                                      event,
                                      "thinking.completed",
                                      "thinking.failed",
                                    )}</small>{/if}
                              </p>{/if}{/each}{#if renderedActivityEvents(request).length > 5 && controller.state.expandedActivity.has(request.event.ref.id)}<p
                              class="event-more"
                            >
                              <span
                                >({renderedActivityEvents(request).length} steps)</span
                               ><button
                                  class="primary inline"
                                  type="button"
                                onclick={() => controller.toggleActivity(request)}
                                 >Show less</button
                              >
                            </p>{/if}
                          </div>{/if}
                    </section>{#each finalReplies(request) as reply (reply.event.ref.id)}<article
                      class="conversation-message card"
                    >
                      <header>
                        {reply.event.author_agent === undefined
                          ? "Gatehouse"
                          : agentLabel(reply.event.author_agent.id)}
                      </header>
                      {#if reply.event.payload.text !== ""}<button
                          class="icon inline small"
                          type="button"
                          aria-label="Copy response Markdown"
                          title="Copy Markdown"
                          onclick={() =>
                            void copyMarkdown(reply.event.payload.text ?? "")}
                          ><Copy size={16} strokeWidth={2} /></button
                        >
                        <div class="prose">
                          {@html renderMarkdown(reply.event.payload.text ?? "")}
                        </div>{:else if reply.event.payload.attachments === undefined || reply.event.payload.attachments.length === 0}<div
                        >
                          <em>No reply.</em>
                        </div>{/if}{#if reply.event.payload.attachments !== undefined && reply.event.payload.attachments.length > 0}<div
                          class="attachment-list"
                          aria-label="Attached files"
                        >
                          {#each reply.event.payload.attachments as file (file.id)}<a
                              class="attachment-chip badge"
                              href={chatFileDownloadPath(
                                workspace.id,
                                session.id,
                                file.id,
                              )}
                              target="_blank"
                              rel="noopener noreferrer"
                              download={file.name}
                              title={file.fingerprint}
                              ><Paperclip
                                size={14}
                                strokeWidth={2}
                                aria-hidden="true"
                              /><span>{file.name}</span><small
                                >{file.size} bytes{file.media_type === undefined
                                  ? ""
                                  : ` · ${file.media_type}`}</small
                              ></a
                            >{/each}
                        </div>{/if}
                    </article>{/each}{/each}{/if}{/each}{/if}{#if controller.state.showJumpToLatest}<button
                class="primary small conversation-jump"
                type="button"
                onclick={() => void controller.jumpToLatest()}
                >Jump to latest</button
              >{/if}
          </div>
        {/if}
      </SidebarPage.Body>
      {#if session !== null}<SidebarPage.Footer placement="floating" surface={false}
          ><form
            class="input-group"
            data-pending={controller.state.sendingMessage || undefined}
            autocomplete="off"
            onsubmit={(event) => {
              event.preventDefault()
              void controller.sendMessage(delivery)
            }}
          >
            <label class="visually-hidden" for="message">Message</label><input
              class="visually-hidden"
              id="files"
              type="file"
              autocomplete="off"
              multiple
              bind:this={fileInputElement}
              onchange={(event) =>
                controller.selectComposerFiles(event.currentTarget)}
            />{#if controller.state.composerFiles.length > 0}<div
                class="attachment-list"
                aria-label="Selected files"
              >
                {#each controller.state.composerFiles as entry (entry.file)}<span
                    class="attachment-chip badge"
                    data-state={entry.status}
                    >{#if entry.status === "uploading"}<span
                        class="spinner"
                        aria-hidden="true"
                      ></span>{:else}<Paperclip
                        size={14}
                        strokeWidth={2}
                        aria-hidden="true"
                      />{/if}<span>{entry.file.name}</span><small
                      >{entry.status === "uploading"
                        ? "Uploading"
                        : entry.status === "failed"
                          ? entry.error
                          : entry.id === undefined
                            ? `${entry.file.size} bytes`
                            : "Ready"}</small
                    ><button
                      class="icon"
                      type="button"
                      aria-label={`Remove ${entry.file.name}`}
                      disabled={controller.state.sendingMessage}
                      onclick={() => controller.removeComposerFile(entry.file)}
                      ><X size={14} strokeWidth={2} /></button
                    ></span
                  >{/each}
              </div>{/if}
            <div class="input-group-controls">
              <button
                class="icon"
                type="button"
                aria-label="Attach files"
                title="Attach files"
                disabled={controller.state.sendingMessage}
                onclick={() => fileInputElement?.click()}
                ><Paperclip
                  size={20}
                  strokeWidth={2.25}
                  aria-hidden="true"
                /></button
              >
              <button
                class="icon"
                type="button"
                aria-label={deliveryTitle}
                title={deliveryTitle}
                data-selected={deliveryMode === "direct" || undefined}
                disabled={controller.state.sendingMessage}
                onclick={openDelivery}
                ><Bot size={20} strokeWidth={2.25} aria-hidden="true" /></button
              >
              <textarea
                id="message"
                rows="1"
                autocomplete="off"
                placeholder="Write a message"
                bind:this={messageInputElement}
                bind:value={controller.state.messageText}
                disabled={controller.state.sendingMessage}
                onkeydown={(event) => {
                  handleMentionKeydown(event)
                  if (event.isComposing) return
                  if (event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault()
                    void controller.sendMessage(delivery)
                  }
                }}
                oninput={openMentionAfterInput}></textarea><button
                class="icon primary"
                type="submit"
                aria-label="Send message"
                title="Send message"
                disabled={controller.state.sendingMessage ||
                  (controller.state.messageText.trim() === "" &&
                    controller.state.composerFiles.length === 0)}
                ><Send
                  size={20}
                  strokeWidth={2.25}
                  aria-hidden="true"
                /></button
                >
            </div>
            {#if controller.state.messageError !== ""}<p
                class="field-help"
                role="alert"
                aria-live="polite"
              >
                {controller.state.messageError}
              </p>{/if}
            <Dialog.Root bind:open={mentionOpen}>
              <Dialog.Portal>
                <Dialog.Overlay
                  class="modal-overlay"
                  onclick={() => (mentionOpen = false)}
                />
                <Dialog.Content
                  class="modal mention-command"
                  preventScroll={false}
                  onOpenAutoFocus={(event) => {
                    event.preventDefault()
                    requestAnimationFrame(() =>
                      mentionSearchInputElement?.focus({ preventScroll: true }),
                    )
                  }}
                  onCloseAutoFocus={(event) => {
                    event.preventDefault()
                    focusComposer()
                  }}
                  onkeydown={(event) => {
                    if (event.key !== "Escape") return
                    event.preventDefault()
                    mentionOpen = false
                    focusComposer()
                  }}
                >
                  <Dialog.Title class="visually-hidden"
                    >Choose an agent</Dialog.Title
                  >
                  <Command.Root label="Choose an agent" shouldFilter={false}>
                    <Command.Input
                      bind:this={mentionSearchInputElement}
                      bind:value={mentionQuery}
                      autofocus
                      placeholder="Search agents"
                    />
                    <Command.List>
                      <Command.Viewport>
                        {#if filteredMentionAgents.length === 0}<p
                            class="mention-command-empty"
                          >No agents found.</p
                        >{:else}<Command.Group>
                          <Command.GroupHeading>Agents</Command.GroupHeading>
                          <Command.GroupItems>
                            {#each filteredMentionAgents as agent (agent.id)}
                              <Command.Item
                                value={agent.id}
                                keywords={[
                                  agent.alias,
                                  ...(agent.label === undefined
                                    ? []
                                    : [agent.label]),
                                ]}
                                onSelect={() => selectMention(agent)}
                                onclick={() => selectMention(agent)}
                              >
                                <strong>@{agent.alias}</strong>
                                {#if agent.label !== undefined}<small
                                    >{agent.label}</small
                                  >{/if}
                              </Command.Item>
                            {/each}
                          </Command.GroupItems>
                        </Command.Group>{/if}
                      </Command.Viewport>
                    </Command.List>
                  </Command.Root>
                </Dialog.Content>
              </Dialog.Portal>
            </Dialog.Root>
            <Dialog.Root bind:open={deliveryOpen}>
              <Dialog.Portal>
                <Dialog.Overlay
                  class="modal-overlay"
                  onclick={() => (deliveryOpen = false)}
                />
                <Dialog.Content
                  class="modal mention-command delivery-command"
                  preventScroll={false}
                  onOpenAutoFocus={(event) => {
                    event.preventDefault()
                    requestAnimationFrame(() =>
                      deliverySearchInputElement?.focus({ preventScroll: true }),
                    )
                  }}
                  onkeydown={(event) => {
                    if (event.key === "Escape") deliveryOpen = false
                  }}
                >
                  <Dialog.Title class="visually-hidden"
                    >Message delivery</Dialog.Title
                  >
                  <Command.Root label="Choose message delivery" shouldFilter={false}>
                    <Command.Input
                      bind:this={deliverySearchInputElement}
                      bind:value={deliveryQuery}
                      autofocus
                      placeholder="Search chat modes"
                    />
                    <Command.List>
                      <Command.Viewport>
                        {#if groupDeliveryVisible || filteredDeliveryAgents.length > 0}<Command.Group>
                            <Command.GroupItems>
                              {#if groupDeliveryVisible}<Command.Item
                                  value="group"
                                  keywords={["group", "human", "explicit"]}
                                  onSelect={setGroupDelivery}
                                >
                                  <strong>Group chat</strong>
                                  <small
                                    >Only explicit @mentions receive a message.</small
                                  >
                                </Command.Item>{/if}{#each filteredDeliveryAgents as agent (agent.id)}<Command.Item
                                  value={`direct-${agent.id}`}
                                  keywords={[agent.alias, ...(agent.label === undefined ? [] : [agent.label]) ]}
                                  onSelect={() => setDirectDelivery(agent.id)}
                                >
                                  <strong>Direct chat</strong>
                                  <small
                                    >@{agent.alias}{agent.label === undefined
                                      ? ""
                                      : ` · ${agent.label}`}</small
                                  >
                                </Command.Item
                              >{/each}
                            </Command.GroupItems>
                          </Command.Group>{:else}<p class="mention-command-empty"
                            >No chat modes found.</p
                          >{/if}
                      </Command.Viewport>
                    </Command.List>
                  </Command.Root>
                </Dialog.Content>
              </Dialog.Portal>
            </Dialog.Root>
          </form></SidebarPage.Footer
        >{/if}
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
