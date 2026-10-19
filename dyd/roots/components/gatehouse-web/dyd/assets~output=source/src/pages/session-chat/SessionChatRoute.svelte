<script lang="ts">
  import {
    ArrowDown,
    Send,
    AtSign,
    CircleCheck,
    CircleX,
    Copy,
    Paperclip,
    ShieldCheck,
    ShieldQuestionMark,
    ShieldX,
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
  import SessionViewNavigation from "../../session-participants/SessionViewNavigation.svelte"
  import { onMount, tick, untrack } from "svelte"
  import { createLoadState } from "../../utils/load-state.svelte"
  import {
    chatFileDownloadPath,
    fetchChatSession,
    type MessageTargetRef,
    type ChatEventTree,
  } from "../../app/chat"
  import { useRuntime } from "../../app/runtime.svelte"

  import InputRequestCard from "./InputRequestCard.svelte"
  import InputRequestActivity from "./InputRequestActivity.svelte"
  import RecipientPicker from "./RecipientPicker.svelte"
  import { createRecipientController } from "./recipient-controller.svelte"
  import { targetKey, targetLabel } from "./message-targets"
  import { agentLabel } from "./agent-attribution"
  import { insertedMentionStart, shouldSubmitComposerKey } from "./composer-input"
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

  let { route }: { route?: SessionRoute } = $props()
  const runtime = useRuntime()
  const sessionLoad = createLoadState()
  const { access, activity, auth } = runtime
  let session = $state<Session | null>(null)
  const sessionStatus = $derived(sessionLoad.status)
  let feedElement = $state<HTMLElement | undefined>()
  let messageInputElement = $state<HTMLTextAreaElement | undefined>()
  let fileInputElement = $state<HTMLInputElement | undefined>()
  let mentionOpen = $state(false)
  let mentionStart = -1
  let generation = 0
  let startedSessionKey = ""
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(route ?? (runtime.state.route as SessionRoute))
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
    const scrollingElement = feedElement
    return (
      scrollingElement === undefined ||
      scrollingElement.scrollHeight -
        scrollingElement.scrollTop -
        scrollingElement.clientHeight <
        64
    )
  }
  async function followLatest(
    behavior: ScrollBehavior = "smooth",
  ): Promise<void> {
    const value = generation
    await tick()
    if (
      value !== generation ||
      abortController?.signal.aborted
    )
      return
    if (
      document.activeElement instanceof Element &&
      document.activeElement.closest("[data-input-form]") !== null
    )
      return
    // Initial event loading replaces the placeholder feed with the real one.
    // Resolve the element after the render tick so the first scroll targets it.
    const scrollingElement = feedElement
    scrollingElement?.scrollTo({
      top: scrollingElement.scrollHeight,
      behavior,
    })
  }
  const controller = untrack(() =>
    createChatController({
      isNearBottom,
      followLatest,
      focusComposer: () => {
        if (runtime.state.route.kind === "session-chat" && !mentionOpen && document.activeElement === messageInputElement)
          messageInputElement?.focus({ preventScroll: true })
      },
    }),
  )
  const recipients = createRecipientController()
  const sendLabel = $derived(
    !controller.state.sendingMessage
      ? "Send message"
      : controller.state.composerFiles.some((entry) => entry.status === "uploading")
        ? "Uploading files"
        : "Sending message",
  )

  const activation = $derived(
    JSON.stringify([
      sessionKey,
      auth.state.status,
      auth.state.claims?.principal.ref.id,
      access.state.workspaceStatus,
      workspace !== null,
    ]),
  )
  $effect(() => {
    activation
    return untrack(() => {
      const [workspaceID, sessionID] = sessionKey.split("\u0000")
      return activate({ workspaceID, sessionID })
    })
  })
  $effect(() => {
    if (controller.state.authenticationRequired) runtime.requireLogin()
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
    const refreshRecipients = () => void recipients.refresh()
    window.addEventListener("focus", refreshRecipients)
    const clock = window.setInterval(
      () => controller.updateActivityTimestamp(),
      1000,
    )
    return () => {
      document.removeEventListener("click", copyCodeBlock)
      window.removeEventListener("focus", refreshRecipients)
      window.clearInterval(clock)
    }
  })

  function activate(route: SessionTarget): () => void {
    const value = ++generation
    sessionLoad.reset()
    startedSessionKey = ""
    abortController?.abort()
    abortController = new AbortController()
    unsubscribe?.()
    unsubscribe = undefined
    controller.stop()
    recipients.stop()
    mentionOpen = false
    mentionStart = -1
    session = null
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
        sessionLoad.stop()
        unsubscribe?.()
        unsubscribe = undefined
        controller.stop()
        recipients.stop()
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
    const key = `${workspaceID}/${sessionID}`
    if (startedSessionKey !== key) {
      startedSessionKey = key
      controller.start(workspaceID, sessionID, signal)
      const principalID = auth.state.claims?.principal.ref.id
      if (principalID)
        recipients.start(principalID, workspaceID, sessionID, signal)
    } else
      await Promise.all([
        controller.refreshEvents(),
        recipients.refresh(),
      ])
    if (!isCurrent(value, workspaceID, sessionID, signal)) return
    subscribe(route, value, signal)
  }

  async function loadSession(
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
    showLoading = true,
  ): Promise<boolean> {
    const request = sessionLoad.begin(
      AbortSignal.any([signal, abortController!.signal]),
    )
    signal = request.signal
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
      return request.commit(() => {
        session = loaded
      })
    } catch {
      if (isCurrent(value, workspaceID, sessionID, signal))
        request.fail(new Error("Chat information could not be loaded."))
      return false
    } finally {
      request.finish()
    }
  }

  function subscribe(
    route: SessionTarget,
    value: number,
    routeSignal: AbortSignal,
  ): void {
    unsubscribe?.()
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
          events: ["workspace_agent.*", "group.*", "group_member.*", "group_agent.*"],
        },
        {
          name: "session-recipients",
          topic: `${workspaceID}/${sessionID}`,
          events: ["session_grant.*"],
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
          ...(names.has("session-chat-agent") || names.has("session-recipients")
            ? [recipients.refresh(signal)] : []),
          ...(names.has("session-chat") || names.has("session-chat-agent")
            ? [
                controller.refreshEvents(signal),
                loadSession(
                  value,
                  workspaceID,
                  sessionID,
                  AbortSignal.any([routeSignal, signal]),
                  false,
                ),
              ]
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
      return request.event.target_agent ? agentLabel(request.event.target_agent) : request.event.payload.agent
    if (tree.event.kind === "agent.request")
      return tree.event.target_agent ? agentLabel(tree.event.target_agent) : tree.event.payload.agent
    const activity = activityEvents(tree).find(
      (child) => child.event.author_agent !== undefined,
    )
    if (activity?.event.author_agent !== undefined)
      return agentLabel(activity.event.author_agent)
    const reply = finalReplies(tree)[0]
    return reply?.event.author_agent === undefined
      ? "Agent"
      : agentLabel(reply.event.author_agent)
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
  function openMentionAfterInput(event: Event, data: string | null, composing = false): void {
    if (mentionOpen || !(event.currentTarget instanceof HTMLTextAreaElement)) return
    const input = event.currentTarget
    const start = insertedMentionStart(input.value, input.selectionStart, data, composing)
    if (start === undefined) return
    mentionStart = start
    mentionOpen = true
    void recipients.refresh()
  }
  function focusComposer(cursor?: number): void {
    messageInputElement?.focus({ preventScroll: true })
    if (cursor !== undefined)
      messageInputElement?.setSelectionRange(cursor, cursor)
  }
  function selectRecipient(ref: MessageTargetRef | null): void {
    recipients.select(ref === null ? [] : [...recipients.state.targets, ref])
    if (mentionStart >= 0 && controller.state.messageText[mentionStart] === "@") {
      const text = controller.state.messageText
      controller.state.messageText = text.slice(0, mentionStart) + text.slice(mentionStart + 1)
    }
    mentionOpen = false
  }
  function openRecipients(): void {
    mentionStart = -1
    mentionOpen = true
    void recipients.refresh()
  }
  async function sendMessage(): Promise<void> {
    if (!recipients.canSend) return
    await controller.sendMessage(recipients.state.targets)
    void recipients.refresh()
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
  <SidebarPage.Root viewport>
    <SidebarPage.Sidebar
      ><UI.Navigation
        {...workspaceNavigation(runtime, workspace, "chats")}
      /></SidebarPage.Sidebar
    >
    <SidebarPage.Page>
      <SidebarPage.Header divider compactBreadcrumb>
        <SidebarPage.Toggle />
        <UI.Breadcrumb.Root aria-label="Breadcrumb" data-page-breadcrumb>
          <UI.Breadcrumb.List>
            <UI.Breadcrumb.Item
              ><RouterLink href={workspacePath(workspace.id)}
                >{workspace.name ?? "Workspace"}</RouterLink
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
        {#if session !== null}<SessionViewNavigation
            workspaceID={workspace.id}
            sessionID={session.id}
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
      <SidebarPage.Body padding="none" scroll="contained">
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
            bind:element={feedElement}
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
          <UI.Feed
            bind:element={feedElement}
            aria-live="polite"
            onscroll={() => controller.trackScroll()}
          >
            {#if sessionLoad.state.error || controller.state.reloadError}<UI.Notice
                variant="destructive"
                role="alert"
                 >{sessionLoad.state.error || controller.state.reloadError}<UI.Button
                  variant="outline"
                  onclick={() =>
                    void loadRoute(
                      currentRoute,
                      generation,
                      abortController!.signal,
                    )}>Retry</UI.Button
                ></UI.Notice
              >{/if}
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
                      {tree.event.author_principal?.name ?? tree.event.author_principal?.alias ?? (tree.event.author_agent ? agentLabel(tree.event.author_agent) : "User")}
                    {/snippet}
                    {#snippet metadata()}
                      {#if tree.event.kind === "message.text"}
                        {@const addressed = tree.event.payload.targets ?? []}
                        {#if addressed.length > 0}
                          <UI.Text as="p" size="small" tone="muted" aria-label="Message recipients">To: {addressed.map(targetLabel).join(", ")}</UI.Text>
                        {/if}
                      {/if}
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
                      gap="small"
                    >
                      <UI.Row as="header" gap="small">
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
                          >{/if}{#if replyCanBeCancelled(request)}<UI.InlineButton
                            type="button"
                            disabled={controller.state.cancellingReplyFor.has(
                              request.event.ref.id,
                            )}
                            onclick={() => void controller.cancelReply(request)}
                            >{controller.state.cancellingReplyFor.has(
                              request.event.ref.id,
                            )
                              ? "Cancelling..."
                              : "Cancel"}</UI.InlineButton
                          >{/if}
                      </UI.Row>
                      {#if renderedActivityEvents(request).length > 0 || agentFailureSummary(request) !== undefined || cancellationRequest(request) !== undefined}<UI.Stack
                          indent
                          gap="small"
                        >
                          {#if renderedActivityEvents(request).length > 5 && !controller.state.expandedActivity.has(request.event.ref.id)}<UI.Row
                              as="p"
                            >
                              <UI.Text
                                >({renderedActivityEvents(request).length - 5} more)</UI.Text
                              ><UI.InlineButton
                                type="button"
                                onclick={() =>
                                  controller.toggleActivity(request)}
                                >Show all</UI.InlineButton
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
                              </UI.Row>
                              {/if}{/each}{#if renderedActivityEvents(request).length > 5 && controller.state.expandedActivity.has(request.event.ref.id)}<UI.Row
                              as="p"
                            >
                              <UI.Text
                                >({renderedActivityEvents(request).length} steps)</UI.Text
                              ><UI.InlineButton
                                type="button"
                                onclick={() =>
                                  controller.toggleActivity(request)}
                                >Show less</UI.InlineButton
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
                        purpose="output"
                      >
                        {#snippet header()}
                          {reply.event.author_agent === undefined
                            ? "Gatehouse"
                            : agentLabel(reply.event.author_agent)}
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
                      </UI.Message>{/each}{/each}{/if}{/each}{/if}
            {#snippet actions()}
              {#if controller.state.showJumpToLatest}
                <UI.FloatingAction
                  variant="default"
                  size="icon-lg"
                  type="button"
                  aria-label="Jump to latest"
                  title="Jump to latest"
                  onclick={() => void controller.jumpToLatest()}
                ><ArrowDown size={20} strokeWidth={2.25} aria-hidden="true" /></UI.FloatingAction>
              {/if}
            {/snippet}
          </UI.Feed>
        {/if}
      </SidebarPage.Body>
      {#if session !== null}<SidebarPage.Footer compact
          ><UI.InputBar
            overlayOpen={mentionOpen}
            data-pending={controller.state.sendingMessage || undefined}
            aria-busy={controller.state.sendingMessage}
            autocomplete="off"
            onsubmit={(event) => {
              event.preventDefault()
              void sendMessage()
            }}
          >
            {#if recipients.state.targets.length > 0}
              <UI.InputBarAttachments label="Message recipients">
                <UI.InlineButton type="button" disabled={controller.state.sendingMessage} onclick={openRecipients}>To:</UI.InlineButton>
                {#each recipients.state.targets as ref (targetKey(ref))}
                  <UI.InputRecipient
                    name={targetLabel(recipients.target(ref))}
                    unavailable={recipients.state.ready && !recipients.available(ref)}
                    pending={recipients.state.ready && recipients.available(ref) && !recipients.inChat(ref)}
                    disabled={controller.state.sendingMessage}
                    onselect={openRecipients}
                    onremove={() => recipients.select(recipients.state.targets.filter((target) => targetKey(target) !== targetKey(ref)))}
                  />
                {/each}
              </UI.InputBarAttachments>
            {/if}
            {#if recipients.state.error || (recipients.state.outsideError && recipients.state.targets.some((ref) => !recipients.inChat(ref)))}
              <UI.Row><UI.Text as="p" size="small" role="alert">{recipients.state.error || recipients.state.outsideError}</UI.Text>
                <UI.Button type="button" variant="outline" size="sm" onclick={() => void recipients.refresh()}>Try again</UI.Button>
              </UI.Row>
            {/if}
            <UI.Label hidden for="message">Message</UI.Label><UI.FileInput
              hidden
              id="files"
              type="file"
              autocomplete="off"
              multiple
              bind:ref={fileInputElement}
              onchange={(event) =>
                controller.selectComposerFiles(event.currentTarget)}
            />
            {#if controller.state.composerFiles.length > 0}
              <UI.InputBarAttachments>
                {#each controller.state.composerFiles as entry (entry.file)}
                  <UI.InputAttachment
                    name={entry.file.name}
                    size={entry.file.size}
                    status={entry.status}
                    uploaded={entry.id !== undefined}
                    error={entry.error}
                    disabled={controller.state.sendingMessage}
                    onremove={() => controller.removeComposerFile(entry.file)}
                  />
                {/each}
              </UI.InputBarAttachments>
            {/if}
            {#snippet input()}
              <UI.Textarea
              mode="composer"
              id="message"
              rows={1}
              enterkeyhint="enter"
              autocomplete="off"
              placeholder="Write a message"
              aria-describedby={controller.state.messageError !== "" ? "message-error" : undefined}
              bind:ref={messageInputElement}
              bind:value={controller.state.messageText}
              onkeydown={(event) => {
                const mobile = window.matchMedia("(max-width: 767px), (pointer: coarse)").matches
                if (shouldSubmitComposerKey(event, mobile)) {
                  event.preventDefault()
                  void sendMessage()
                }
              }}
              oninput={(event) => {
                if (event instanceof InputEvent)
                  openMentionAfterInput(event, event.data, event.isComposing)
              }}
              oncompositionend={(event) => openMentionAfterInput(event, event.data)}
            />
            {/snippet}
            {#snippet leading()}
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
                  variant="ghost"
                  size="icon"
                  type="button"
                  aria-label="Choose recipient"
                  title="Choose recipient"
                  disabled={controller.state.sendingMessage}
                  onclick={() => openRecipients()}
                  ><AtSign
                    size={20}
                    strokeWidth={2.25}
                    aria-hidden="true"
                  /></UI.Button
                >
            {/snippet}
            {#snippet trailing()}
              <UI.Button
                variant="default"
                size="icon"
                type="submit"
                aria-label={sendLabel}
                title={sendLabel}
                disabled={controller.state.sendingMessage ||
                  !recipients.canSend ||
                  (controller.state.messageText.trim() === "" &&
                    controller.state.composerFiles.length === 0)}
                >{#if controller.state.sendingMessage}
                  <UI.Spinner aria-hidden="true" />
                {:else}
                  <Send size={20} strokeWidth={2.25} aria-hidden="true" />
                {/if}</UI.Button
              >
            {/snippet}
            {#snippet feedback()}
            {#if controller.state.messageError !== ""}<UI.Text
                id="message-error"
                as="p"
                size="small"
                role="alert"
                aria-live="polite"
              >
                {controller.state.messageError}
              </UI.Text>{/if}
            {/snippet}
            <RecipientPicker
              bind:open={mentionOpen}
              inChat={recipients.state.inChat}
              outside={recipients.state.outside}
              canManage={recipients.state.canManage}
              ready={recipients.state.ready}
              error={recipients.state.error}
              outsideReady={recipients.state.outsideReady}
              outsideError={recipients.state.outsideError}
              onselect={selectRecipient}
              onretry={() => void recipients.refresh()}
              onclose={() => {
                focusComposer(mentionStart >= 0 ? mentionStart : undefined)
                mentionStart = -1
              }}
            />
          </UI.InputBar></SidebarPage.Footer
        >{/if}
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
