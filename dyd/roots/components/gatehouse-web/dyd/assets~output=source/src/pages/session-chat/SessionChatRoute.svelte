<script lang="ts">
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
    Command,
    Dialog,
    RouterLink,
    StatusPage,
    renderMarkdown,
  } from "../../ui"
  import {
    workspaceNavigation,
    sessionViews,
    renameSession,
  } from "../../app/navigation"
  import * as UI from "../../ui"
  import { onMount, tick, untrack } from "svelte"
  import {
    chatFileDownloadPath,
    fetchChatSession,
    type ChatAgent,
    type ChatEventTree,
  } from "../../app/chat"
  import { useRuntime } from "../../app/runtime.svelte"

  import InputRequestCard from "./InputRequestCard.svelte"
  import InputRequestActivity from "./InputRequestActivity.svelte"
  import * as SidebarPage from "../../ui/sidebar-page"

  import type { Route } from "../../route"
  import {
    activityDuration,
    activityEvents,
    agentFailures,
    agentRequests,
    approvalRequests,
    approvalResponse,
    cancellationRequest,
    type ChatDelivery,
    createChatController,
    displayedActivityEvents,
    elapsedDuration,
    finalReplies,
    hasCancellationFailure,
    hasCancellationSuccess,
    inputRequests,
    pendingReplyInputRequests,
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
    if (
      document.activeElement instanceof Element &&
      document.activeElement.closest("[data-input-form]") !== null
    )
      return
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
  let directAgentID = $state<string | null>(untrack(() => currentRoute.agent))
  let deliveryMode = $state<ChatDelivery["mode"]>(
    untrack(() => currentRoute.mode),
  )
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
      const button = event.target.closest<HTMLButtonElement>("[data-copy-code]")
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
  function agentFailureSummary(tree: ChatEventTree): string | undefined {
    const failure = agentFailures(tree)[0]?.event.payload
    if (failure === undefined) return undefined
    if (
      hasCancellationSuccess(tree) &&
      failure.code === "cancelled" &&
      (failure.message === undefined || failure.message === "cancelled")
    )
      return undefined
    return failure.message ?? failure.code
  }
  function thinkingSummary(tree: ChatEventTree): string {
    const status = thinkingStatus(tree)
    if (status === "succeeded") return "Thought"
    if (status === "failed")
      return tree.children.some(
        (child) =>
          child.event.kind === "thinking.failure" &&
          child.event.payload.code === "cancelled",
      )
        ? "Thinking cancelled after"
        : "Thinking failed after"
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
    if (tree.event.kind === "agent.request")
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
    approval: ChatEventTree<"approval.request">,
    task: ChatEventTree<"tool.request" | "thinking.request">,
  ): string {
    return (
      approval.event.payload.description ??
      (task.event.kind === "tool.request"
        ? task.event.payload.reason
        : undefined) ??
      `Run ${task.event.kind === "tool.request" ? (task.event.payload.name ?? "tool") : "tool"}`
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
    const suffix = text.slice(start + 1)
    controller.state.messageText =
      text.slice(0, start) +
      alias +
      (suffix.startsWith(" ") ? "" : " ") +
      suffix
    mentionOpen = false
    focusComposer(start + alias.length + 1)
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
      <UI.Text as="p">
        {auth.state.status === "checking"
          ? "Checking your session."
          : "Loading your workspaces."}
      </UI.Text>
    {/snippet}
  </StatusPage>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage
    eyebrow="Gatehouse"
    title="Connection unavailable"
    description="Gatehouse could not load your account."
  >
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => void runtime.refresh()}>Try again</UI.Button
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else if auth.state.status !== "authenticated"}
  <StatusPage eyebrow="Gatehouse" title="Sign in required">
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => runtime.requireLogin()}>Sign in</UI.Button
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <StatusPage
    eyebrow="Gatehouse"
    title="No workspace access"
    description="Ask an administrator to add you to a workspace group."
  >
    {#snippet children()}<UI.Group
        ><UI.Button
          variant="default"
          type="button"
          onclick={() => runtime.navigate("/app/no-access", true)}
          >Continue</UI.Button
        ></UI.Group
      >{/snippet}
  </StatusPage>
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar
      ><UI.Navigation
        {...workspaceNavigation(runtime, workspace, "chats")}
      /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header placement="floating">
        <SidebarPage.Toggle />
        <UI.Breadcrumb.Root aria-label="Breadcrumb" data-page-breadcrumb>
          <UI.Breadcrumb.List>
            <UI.Breadcrumb.Item
              ><RouterLink href={workspacePath(workspace.id)}
                >{workspace.name ?? workspace.id}</RouterLink
              ></UI.Breadcrumb.Item
            >
            {#if session !== null}
              {#if session.project !== undefined}
                <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item
                  ><RouterLink
                    href={projectPath(workspace.id, session.project.id)}
                    >{session.project.name ?? "New Project"}</RouterLink
                  ></UI.Breadcrumb.Item
                >
              {/if}
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
                ><UI.Breadcrumb.Page
                  >{session.name ?? "New Chat"}</UI.Breadcrumb.Page
                ></UI.Breadcrumb.Item
              >
            {/if}
          </UI.Breadcrumb.List>
        </UI.Breadcrumb.Root>
        {#if session !== null}<UI.ViewNavigation
            items={sessionViews(workspace.id, session.id, "chat")}
            name={session.name}
            onRename={(name) =>
              renameSession(
                runtime,
                workspace.id,
                currentRoute.sessionID,
                name,
                updateSessionName,
              )}
          />{/if}
      </SidebarPage.Header>
      <SidebarPage.Body>
        {#if sessionStatus === "unavailable"}<UI.Text as="p" tone="muted">
            This chat could not be loaded.
          </UI.Text>
          <UI.Button
            variant="default"
            type="button"
            onclick={() =>
              void loadRoute(currentRoute, generation, abortController!.signal)}
            >Try again</UI.Button
          >
        {:else if sessionStatus === "checking" || controller.state.status === "checking"}<UI.Feed
            data-loading
            aria-busy="true"
            aria-live="polite"
          >
            <UI.Text as="p" tone="muted"
              ><UI.Spinner role="status"
                ><UI.Text hidden>Loading</UI.Text></UI.Spinner
              > Loading chat...</UI.Text
            >
          </UI.Feed>
        {:else if session !== null}
          <UI.Feed aria-live="polite">
            {#if controller.state.status === "unavailable"}<UI.Text
                as="p"
                tone="muted"
              >
                This chat could not be loaded.
              </UI.Text>{:else if controller.state.events.length === 0}<UI.Text
                as="p"
                tone="muted"
              >
                Send the first message to begin.
              </UI.Text>{:else}{#each controller.state.events as tree (tree.event.ref.id)}{#if tree.event.kind === "message.text" && (tree.event.payload.text !== undefined || (tree.event.payload.attachments !== undefined && tree.event.payload.attachments.length > 0))}<UI.Message
                    align="end"
                  >
                    {#snippet header()}
                      {tree.event.author_principal?.name ?? "User"}
                    {/snippet}
                    {#snippet actions()}{#if tree.event.kind === "message.text" && tree.event.payload.text !== undefined}<UI.Button
                          variant="ghost"
                          size="icon-sm"
                          type="button"
                          aria-label="Copy message Markdown"
                          title="Copy Markdown"
                          onclick={() =>
                            void copyMarkdown(
                              "text" in tree.event.payload
                                ? (tree.event.payload.text ?? "")
                                : "",
                            )}><Copy size={16} strokeWidth={2} /></UI.Button
                        >
                      {/if}{/snippet}
                    {#if tree.event.payload.text !== undefined}
                      <UI.RichText
                        html={renderMarkdown(tree.event.payload.text)}

                      ></UI.RichText>{/if}{#if tree.event.payload.attachments !== undefined && tree.event.payload.attachments.length > 0}<UI.Row
                        aria-label="Attached files"
                      >
                        {#each tree.event.payload.attachments as file (file.id)}<UI.Chip
                            href={chatFileDownloadPath(
                              workspace.id,
                              session.id,
                              file.id,
                            )}
                            target="_blank"
                            rel="noopener noreferrer"
                            download={file.name}
                            title={file.fingerprint}
                            ><UI.Text
                              ><Paperclip
                                size={14}
                                strokeWidth={2}
                                aria-hidden="true"
                              /></UI.Text
                            ><UI.Row as="span"
                              ><UI.Text>{file.name}</UI.Text><UI.Text
                                as="small"
                                size="small"
                                >{file.size} bytes{file.media_type === undefined
                                  ? ""
                                  : ` · ${file.media_type}`}</UI.Text
                              ></UI.Row
                            ></UI.Chip
                          >{/each}
                      </UI.Row>{/if}
                  </UI.Message>
                  {#each agentRequests(tree) as request (request.event.ref.id)}<UI.Stack
                      as="section"
                    >
                      <UI.Group as="header">
                        {cancellationRequest(request) !== undefined
                          ? activityAgentLabel(request)
                          : agentFailures(request).length > 0
                            ? `${activityAgentLabel(request)} failed`
                            : finalReplies(request).length === 0
                              ? `${activityAgentLabel(request)} is working`
                              : activityAgentLabel(
                                  request,
                                )}{#if finalReplies(request).length > 0 && replyDuration(request) !== ""}<UI.Text
                            as="small"
                            size="small">{replyDuration(request)}</UI.Text
                          >{/if}{#if replyCanBeCancelled(request) && workingReplyDuration(request) !== ""}<UI.Text
                            as="small"
                            size="small"
                            >{workingReplyDuration(request)}</UI.Text
                          >{/if}{#if replyCanBeCancelled(request)}<UI.Button
                            variant="default"
                            type="button"
                            disabled={controller.state.cancellingReplyFor.has(
                              request.event.ref.id,
                            )}
                            onclick={() => void controller.cancelReply(request)}
                            >{controller.state.cancellingReplyFor.has(
                              request.event.ref.id,
                            )
                              ? "Cancelling..."
                              : "Cancel"}</UI.Button
                          >{/if}
                      </UI.Group>
                      {#if renderedActivityEvents(request).length > 0 || agentFailureSummary(request) !== undefined || cancellationRequest(request) !== undefined}<UI.Stack
                          indent
                        >
                          {#if renderedActivityEvents(request).length > 5 && !controller.state.expandedActivity.has(request.event.ref.id)}<UI.Row
                              as="p"
                            >
                              <UI.Text
                                >({renderedActivityEvents(request).length - 5} more)</UI.Text
                              ><UI.Button
                                variant="default"
                                type="button"
                                onclick={() =>
                                  controller.toggleActivity(request)}
                                >Show all</UI.Button
                              >
                            </UI.Row>{/if}{#each displayedActivityEvents(request, controller.state.expandedActivity) as event (event.event.ref.id)}{#if event.event.kind === "tool.request"}<UI.Row
                                tone="muted"
                                as="p"
                                data-state={toolStatus(event)}
                                title={event.event.payload.name ?? "tool"}
                              >
                                {#if toolStatus(event) === "working"}<UI.Spinner
                                    aria-hidden="true"

                                  ></UI.Spinner>{:else if toolStatus(event) === "succeeded"}<CircleCheck
                                    size={14}
                                    strokeWidth={2}
                                    aria-hidden="true"
                                  />{:else}<CircleX
                                    size={14}
                                    strokeWidth={2}
                                    aria-hidden="true"
                                  />{/if}Task: {event.event.payload.reason ??
                                  `Running ${event.event.payload.name ?? "tool"}`}{#if activityDuration(event, "tool.success", "tool.failure") !== ""}<UI.Text
                                    as="small"
                                    size="small"
                                    >{activityDuration(
                                      event,
                                      "tool.success",
                                      "tool.failure",
                                    )}</UI.Text
                                  >{/if}
                              </UI.Row>
                              {#each approvalRequests(event) as approval (approval.event.ref.id)}{@const response =
                                  approvalResponse(approval)}
                                <UI.Row
                                  tone="muted"
                                  data-resolved={response !== undefined ||
                                    undefined}
                                >
                                  {#if response === undefined}<ShieldQuestionMark
                                      size={15}
                                      strokeWidth={2}
                                      aria-hidden="true"
                                    /><UI.Text as="strong" emphasis="strong"
                                      >Action approval required:</UI.Text
                                    ><UI.Text
                                      >{approvalDescription(
                                        approval,
                                        event,
                                      )}</UI.Text
                                    ><UI.Text data-actions
                                      ><UI.Button
                                        variant="default"
                                        size="sm"
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
                                          : "Approve"}</UI.Button
                                      ><UI.Button
                                        variant="outline"
                                        size="sm"
                                        type="button"
                                        disabled={controller.state.submittingApprovals.has(
                                          approval.event.ref.id,
                                        )}
                                        onclick={() =>
                                          void controller.respondToApproval(
                                            approval,
                                            "rejected",
                                          )}>Reject</UI.Button
                                      ></UI.Text
                                    >{:else if response.event.kind === "approval.success"}<ShieldCheck
                                      size={15}
                                      strokeWidth={2}
                                      aria-hidden="true"
                                    /><UI.Text as="strong" emphasis="strong"
                                      >Action approved:</UI.Text
                                    ><UI.Text
                                      >{approvalDescription(
                                        approval,
                                        event,
                                      )}</UI.Text
                                    >{:else}<ShieldX
                                      size={15}
                                      strokeWidth={2}
                                      aria-hidden="true"
                                    /><UI.Text as="strong" emphasis="strong"
                                      >{response.event.payload.code ===
                                      "cancelled"
                                        ? "Action cancelled:"
                                        : "Action rejected:"}</UI.Text
                                    ><UI.Text
                                      >{approvalDescription(
                                        approval,
                                        event,
                                      )}</UI.Text
                                    >{/if}
                                </UI.Row>
                                {#if response === undefined && (controller.state.approvalErrors.get(approval.event.ref.id) ?? "") !== ""}<UI.Row
                                    tone="muted"
                                    as="p"
                                    role="alert"
                                  >
                                    {controller.state.approvalErrors.get(
                                      approval.event.ref.id,
                                    )}
                                  </UI.Row>{/if}{/each}
                              {#each inputRequests(event) as input (input.event.ref.id)}
                                <InputRequestActivity request={input} />
                              {/each}{:else if event.event.kind === "thinking.request"}<UI.Row
                                tone="muted"
                                as="p"
                                data-state={thinkingStatus(event)}
                              >
                                {#if thinkingStatus(event) === "working"}<UI.Spinner
                                    aria-hidden="true"

                                  ></UI.Spinner>{:else if thinkingStatus(event) === "succeeded"}<CircleCheck
                                    size={14}
                                    strokeWidth={2}
                                    aria-hidden="true"
                                  />{:else}<CircleX
                                    size={14}
                                    strokeWidth={2}
                                    aria-hidden="true"
                                  />{/if}{thinkingSummary(
                                  event,
                                )}{#if activityDuration(event, "thinking.success", "thinking.failure") !== ""}<UI.Text
                                    as="small"
                                    size="small"
                                    >{activityDuration(
                                      event,
                                      "thinking.success",
                                      "thinking.failure",
                                    )}</UI.Text
                                  >{/if}
                              </UI.Row>{/if}{/each}{#if renderedActivityEvents(request).length > 5 && controller.state.expandedActivity.has(request.event.ref.id)}<UI.Row
                              as="p"
                            >
                              <UI.Text
                                >({renderedActivityEvents(request).length} steps)</UI.Text
                              ><UI.Button
                                variant="default"
                                type="button"
                                onclick={() =>
                                  controller.toggleActivity(request)}
                                >Show less</UI.Button
                              >
                            </UI.Row>{/if}
                          {#if cancellationRequest(request) !== undefined}
                            <UI.Row tone="muted" as="p" role="status">
                              {#if hasCancellationSuccess(request)}
                                <CircleX
                                  size={14}
                                  strokeWidth={2}
                                  aria-hidden="true"
                                />
                                <UI.Text>Cancelled</UI.Text>
                              {:else if hasCancellationFailure(request)}
                                <CircleX
                                  size={14}
                                  strokeWidth={2}
                                  aria-hidden="true"
                                />
                                <UI.Text>Cancellation failed</UI.Text>
                              {:else}
                                <UI.Spinner aria-hidden="true"></UI.Spinner>
                                <UI.Text>Cancellation requested</UI.Text>
                              {/if}
                            </UI.Row>
                          {/if}
                          {#if agentFailureSummary(request) !== undefined}<UI.Row
                              tone="muted"
                              as="p"
                              data-state="failed"
                              ><CircleX
                                size={14}
                                strokeWidth={2}
                                aria-hidden="true"
                              /><UI.Text>{agentFailureSummary(request)}</UI.Text
                              ></UI.Row
                            >{/if}
                        </UI.Stack>{/if}
                    </UI.Stack>
                    {#each pendingReplyInputRequests(request) as input (input.event.ref.id)}
                      <InputRequestCard
                        request={input}
                        workspaceID={workspace.id}
                        sessionID={session.id}
                        onAuthenticationLost={runtime.requireLogin}
                        onResolved={() => void controller.refreshEvents()}
                      />
                    {/each}
                    {#each finalReplies(request) as reply (reply.event.ref.id)}<UI.Message
                      >
                        {#snippet header()}
                          {reply.event.author_agent === undefined
                            ? "Gatehouse"
                            : agentLabel(reply.event.author_agent.id)}
                        {/snippet}
                        {#snippet actions()}{#if reply.event.payload.text !== ""}<UI.Button
                              variant="ghost"
                              size="icon-sm"
                              type="button"
                              aria-label="Copy response Markdown"
                              title="Copy Markdown"
                              onclick={() =>
                                void copyMarkdown(
                                  reply.event.payload.text ?? "",
                                )}><Copy size={16} strokeWidth={2} /></UI.Button
                            >
                          {/if}{/snippet}
                        {#if reply.event.payload.text !== ""}
                          <UI.RichText
                            html={renderMarkdown(
                              reply.event.payload.text ?? "",
                            )}

                          ></UI.RichText>{:else if reply.event.payload.attachments === undefined || reply.event.payload.attachments.length === 0}<UI.Group
                          >
                            <UI.Text as="em">No reply.</UI.Text>
                          </UI.Group>{/if}{#if reply.event.payload.attachments !== undefined && reply.event.payload.attachments.length > 0}<UI.Row
                            aria-label="Attached files"
                          >
                            {#each reply.event.payload.attachments as file (file.id)}<UI.Chip
                                href={chatFileDownloadPath(
                                  workspace.id,
                                  session.id,
                                  file.id,
                                )}
                                target="_blank"
                                rel="noopener noreferrer"
                                download={file.name}
                                title={file.fingerprint}
                                ><UI.Text
                                  ><Paperclip
                                    size={14}
                                    strokeWidth={2}
                                    aria-hidden="true"
                                  /></UI.Text
                                ><UI.Row as="span"
                                  ><UI.Text>{file.name}</UI.Text><UI.Text
                                    as="small"
                                    size="small"
                                    >{file.size} bytes{file.media_type ===
                                    undefined
                                      ? ""
                                      : ` · ${file.media_type}`}</UI.Text
                                  ></UI.Row
                                ></UI.Chip
                              >{/each}
                          </UI.Row>{/if}
                      </UI.Message>{/each}{/each}{/if}{/each}{/if}{#if controller.state.showJumpToLatest}<UI.FloatingAction
                variant="default"
                size="sm"
                type="button"
                onclick={() => void controller.jumpToLatest()}
                >Jump to latest</UI.FloatingAction
              >{/if}
          </UI.Feed>
        {/if}
      </SidebarPage.Body>
      {#if session !== null}<SidebarPage.Footer
          placement="floating"
          surface={false}
          ><UI.InputBar
            data-pending={controller.state.sendingMessage || undefined}
            autocomplete="off"
            onsubmit={(event) => {
              event.preventDefault()
              void controller.sendMessage(delivery)
            }}
          >
            <UI.Label hidden for="message">Message</UI.Label><UI.FileInput
              hidden
              id="files"
              type="file"
              autocomplete="off"
              multiple
              bind:ref={fileInputElement}
              onchange={(event) =>
                controller.selectComposerFiles(event.currentTarget)}
            />{#if controller.state.composerFiles.length > 0}<UI.Row
                aria-label="Selected files"
              >
                {#each controller.state.composerFiles as entry (entry.file)}<UI.Chip
                    data-state={entry.status}
                    ><UI.Text
                      >{#if entry.status === "uploading"}<UI.Spinner
                          aria-hidden="true"
                        ></UI.Spinner>{:else}<Paperclip
                          size={14}
                          strokeWidth={2}
                          aria-hidden="true"
                        />{/if}</UI.Text
                    ><UI.Row as="span"
                      ><UI.Text>{entry.file.name}</UI.Text><UI.Text
                        as="small"
                        size="small"
                        >{entry.status === "uploading"
                          ? "Uploading"
                          : entry.status === "failed"
                            ? entry.error
                            : entry.id === undefined
                              ? `${entry.file.size} bytes`
                              : "Ready"}</UI.Text
                      ></UI.Row
                    ><UI.Button
                      variant="ghost"
                      size="icon"
                      type="button"
                      aria-label={`Remove ${entry.file.name}`}
                      disabled={controller.state.sendingMessage}
                      onclick={() => controller.removeComposerFile(entry.file)}
                      ><X size={14} strokeWidth={2} /></UI.Button
                    ></UI.Chip
                  >{/each}
              </UI.Row>{/if}
            <UI.Row wrap={false} align="end">
              <UI.Button
                variant="ghost"
                size="icon"
                type="button"
                aria-label="Attach files"
                title="Attach files"
                disabled={controller.state.sendingMessage}
                onclick={() => fileInputElement?.click()}
                ><Paperclip
                  size={20}
                  strokeWidth={2.25}
                  aria-hidden="true"
                /></UI.Button
              >
              <UI.Button
                variant={deliveryMode === "direct" ? "default" : "outline"}
                size="icon"
                type="button"
                aria-label={deliveryTitle}
                title={deliveryTitle}
                data-selected={deliveryMode === "direct" || undefined}
                disabled={controller.state.sendingMessage}
                onclick={openDelivery}
                ><Bot
                  size={20}
                  strokeWidth={2.25}
                  aria-hidden="true"
                /></UI.Button
              >
              <UI.Textarea
                mode="inline"
                id="message"
                rows={1}
                autocomplete="off"
                placeholder="Write a message"
                bind:ref={messageInputElement}
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
                oninput={openMentionAfterInput}
              ></UI.Textarea><UI.Button
                variant="default"
                size="icon"
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
                /></UI.Button
              >
            </UI.Row>
            {#if controller.state.messageError !== ""}<UI.Text
                as="p"
                size="small"
                role="alert"
                aria-live="polite"
              >
                {controller.state.messageError}
              </UI.Text>{/if}
            <Dialog.Root bind:open={mentionOpen}>
              <Dialog.Content
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
                <UI.DialogTitle hidden>Choose an agent</UI.DialogTitle>
                <Command.Root label="Choose an agent" shouldFilter={false}>
                  <Command.Input
                    bind:ref={mentionSearchInputElement}
                    bind:value={mentionQuery}
                    autofocus
                    placeholder="Search agents"
                  />
                  <Command.List>
                    {#if filteredMentionAgents.length === 0}<UI.Text as="p"
                        >No agents found.</UI.Text
                      >{:else}<Command.Group>
                        <UI.Text size="eyebrow">Agents</UI.Text>

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
                            <UI.Text as="strong" emphasis="strong"
                              >@{agent.alias}</UI.Text
                            >
                            {#if agent.label !== undefined}<UI.Text
                                as="small"
                                size="small">{agent.label}</UI.Text
                              >{/if}
                          </Command.Item>
                        {/each}
                      </Command.Group>{/if}
                  </Command.List>
                </Command.Root>
              </Dialog.Content>
            </Dialog.Root>
            <Dialog.Root bind:open={deliveryOpen}>
              <Dialog.Content
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
                <UI.DialogTitle hidden>Message delivery</UI.DialogTitle>
                <Command.Root
                  label="Choose message delivery"
                  shouldFilter={false}
                >
                  <Command.Input
                    bind:ref={deliverySearchInputElement}
                    bind:value={deliveryQuery}
                    autofocus
                    placeholder="Search chat modes"
                  />
                  <Command.List>
                    {#if groupDeliveryVisible || filteredDeliveryAgents.length > 0}<Command.Group
                      >
                        {#if groupDeliveryVisible}<Command.Item
                            value="group"
                            keywords={["group", "human", "explicit"]}
                            onSelect={setGroupDelivery}
                          >
                            <UI.Text as="strong" emphasis="strong"
                              >Group chat</UI.Text
                            >
                            <UI.Text as="small" size="small"
                              >Only explicit @mentions receive a message.</UI.Text
                            >
                          </Command.Item>{/if}{#each filteredDeliveryAgents as agent (agent.id)}<Command.Item
                            value={`direct-${agent.id}`}
                            keywords={[
                              agent.alias,
                              ...(agent.label === undefined
                                ? []
                                : [agent.label]),
                            ]}
                            onSelect={() => setDirectDelivery(agent.id)}
                          >
                            <UI.Text as="strong" emphasis="strong"
                              >Direct chat</UI.Text
                            >
                            <UI.Text as="small" size="small"
                              >@{agent.alias}{agent.label === undefined
                                ? ""
                                : ` · ${agent.label}`}</UI.Text
                            >
                          </Command.Item>{/each}
                      </Command.Group>{:else}<UI.Text as="p"
                        >No chat modes found.</UI.Text
                      >{/if}
                  </Command.List>
                </Command.Root>
              </Dialog.Content>
            </Dialog.Root>
          </UI.InputBar></SidebarPage.Footer
        >{/if}
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
