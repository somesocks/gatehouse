<script lang="ts">
  import { onMount, tick, untrack } from "svelte"
  import {
    Bot,
    CircleCheck,
    CircleX,
    Copy,
    Menu,
    Paperclip,
    Send,
    ShieldCheck,
    ShieldQuestionMark,
    ShieldX,
    X,
  } from "@lucide/svelte"
  import {
    chatFileDownloadPath,
    fetchChatSession,
    type ChatEventTree,
  } from "../../app/chat"
  import { useRuntime } from "../../app/runtime.svelte"
  import RouterLink from "../../components/RouterLink.svelte"
  import SessionNavigation from "../../components/SessionNavigation.svelte"
  import * as SidebarPage from "../../components/sidebar-page"
  import WorkspaceNavigation from "../../components/WorkspaceNavigation.svelte"
  import { renderMarkdown } from "../../markdown"
  import type { Route } from "../../route"
  import {
    activityDuration,
    activityEvents,
    approvalRequests,
    approvalResponse,
    cancellationRequest,
    createChatController,
    displayedActivityEvents,
    elapsedDuration,
    finalReplies,
    hasCancellationSuccess,
    renderedActivityEvents,
    replyCanBeCancelled,
    thinkingStatus,
    toolStatus,
  } from "./chat-controller.svelte"

  type SessionRoute = Extract<Route, { kind: "session-chat" }>
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
  let chatPageElement = $state<HTMLElement | undefined>()
  let messageInputElement = $state<HTMLTextAreaElement | undefined>()
  let fileInputElement = $state<HTMLInputElement | undefined>()
  let generation = 0
  let abortController: AbortController | null = null
  let unsubscribe: (() => void) | undefined

  const currentRoute = $derived(runtime.state.route as SessionRoute)
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

  function isNearBottom(): boolean {
    return (
      chatPageElement === undefined ||
      chatPageElement.scrollHeight -
        chatPageElement.scrollTop -
        chatPageElement.clientHeight <
        64
    )
  }
  async function followLatest(
    behavior: ScrollBehavior = "smooth",
  ): Promise<void> {
    await tick()
    chatPageElement?.scrollTo({
      top: chatPageElement.scrollHeight,
      behavior,
    })
  }
  function resizeComposer(input = messageInputElement): void {
    if (input === undefined) return
    if (CSS.supports("field-sizing", "content")) return
    input.style.height = "auto"
    const styles = window.getComputedStyle(input)
    const lineHeight =
      Number.parseFloat(styles.lineHeight) ||
      Number.parseFloat(styles.fontSize) * 1.5
    const maximumHeight =
      lineHeight * 6 +
      Number.parseFloat(styles.paddingTop) +
      Number.parseFloat(styles.paddingBottom)
    input.style.height = `${Math.min(input.scrollHeight, maximumHeight)}px`
    input.style.overflowY =
      input.scrollHeight > maximumHeight ? "auto" : "hidden"
  }
  const controller = untrack(() =>
    createChatController({
      isNearBottom,
      followLatest,
      resizeComposer,
      focusComposer: () => messageInputElement?.focus(),
    }),
  )

  $effect(() => {
    const route = currentRoute
    return activate(route)
  })
  $effect(() => {
    if (controller.state.authenticationRequired) runtime.requireLogin()
  })
  $effect(() => {
    const target = chatPageElement
    if (target === undefined) return
    const track = () => controller.trackScroll()
    target.addEventListener("scroll", track)
    return () => target.removeEventListener("scroll", track)
  })

  onMount(() => {
    const copyCodeBlock = (event: MouseEvent) => {
      if (!(event.target instanceof Element)) return
      const button = event.target.closest<HTMLButtonElement>(
        ".markdown-code-copy",
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

  function activate(route: SessionRoute): () => void {
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
    route: SessionRoute,
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
    route: SessionRoute,
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
  function activityAgentLabel(tree: ChatEventTree): string {
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
</script>

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <main class="status-page" aria-busy="true" aria-live="polite">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <div class="loading-mark" aria-hidden="true"></div>
      <p>
        {auth.state.status === "checking"
          ? "Checking your session."
          : "Loading your workspaces."}
      </p>
    </section>
  </main>
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <main class="status-page">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">Connection unavailable</h1>
      <p class="subtitle is-6">Gatehouse could not load your account.</p>
      <button
        class="button is-primary"
        type="button"
        onclick={() => void runtime.refresh()}>Try again</button
      >
    </section>
  </main>
{:else if auth.state.status !== "authenticated"}
  <main class="status-page">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">Sign in required</h1>
      <button
        class="button is-primary"
        type="button"
        onclick={() => runtime.requireLogin()}>Sign in</button
      >
    </section>
  </main>
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <main class="status-page">
    <section class="status-card">
      <p class="eyebrow">Gatehouse</p>
      <h1 class="title is-3">No workspace access</h1>
      <p class="subtitle is-6">
        Ask an administrator to add you to a workspace group.
      </p>
      <button
        class="button is-primary"
        type="button"
        onclick={() => runtime.navigate("/app/no-access", true)}
        >Continue</button
      >
    </section>
  </main>
{:else}
  <SidebarPage.Root>
    <SidebarPage.Sidebar><WorkspaceNavigation {workspace} active="chats" /></SidebarPage.Sidebar>
    <SidebarPage.Page bind:element={chatPageElement}>
      <SidebarPage.Header placement="floating">
        <SidebarPage.Toggle><button class="mobile-menu-trigger" type="button" aria-label="Open navigation menu"><Menu size={20} strokeWidth={2} aria-hidden="true" /></button></SidebarPage.Toggle>
        <h1 class="brand-workspace-breadcrumb">
          <RouterLink class="brand-workspace-breadcrumb-segment" href={workspacePath(workspace.id)}><span>{workspace.name ?? workspace.id}</span></RouterLink>{#if session !== null}{#if session.project !== undefined}<span class="brand-workspace-breadcrumb-separator" aria-hidden="true">/</span><RouterLink class="brand-workspace-breadcrumb-segment" href={projectPath(workspace.id, session.project.id)}><span>{session.project.name ?? "New Project"}</span></RouterLink>{/if}<span class="brand-workspace-breadcrumb-separator" aria-hidden="true">/</span><span class="brand-workspace-breadcrumb-segment">{session.name ?? "New Chat"}</span>{/if}
        </h1>
        {#if session !== null}<SessionNavigation workspaceID={workspace.id} sessionID={session.id} active="chat" />{/if}
      </SidebarPage.Header>
      <SidebarPage.Body>
    {#if sessionStatus === "checking"}<p
        class="dashboard-empty"
        aria-busy="true"
        aria-live="polite"
      >
        Loading chat...
      </p>
    {:else if sessionStatus === "unavailable"}<p class="dashboard-empty">
        This chat could not be loaded.
      </p>
      <button
        class="button is-primary"
        type="button"
        onclick={() =>
          void loadRoute(currentRoute, generation, abortController!.signal)}
        >Try again</button
      >
    {:else if session !== null}
      <div class="chat-events" aria-live="polite">
          {#if controller.state.status === "checking"}<p class="chat-status">
              Loading chat...
            </p>{:else if controller.state.status === "unavailable"}<p
              class="chat-status"
            >
              This chat could not be loaded.
            </p>{:else if controller.state.events.length === 0}<p
              class="chat-status"
            >
              Send the first message to begin.
            </p>{:else}{#each controller.state.events as tree (tree.event.ref.id)}{#if tree.event.kind === "message.text" && (tree.event.payload.text !== undefined || (tree.event.payload.attachments !== undefined && tree.event.payload.attachments.length > 0))}<article
                  class="chat-message message-own"
                >
                  <p class="chat-message-author">
                    {tree.event.author_principal?.name ?? "User"}
                  </p>
                  {#if tree.event.payload.text !== undefined}<button
                      class="chat-message-copy"
                      type="button"
                      aria-label="Copy message Markdown"
                      title="Copy Markdown"
                      onclick={() => void copyMarkdown(tree.event.payload.text)}
                      ><Copy size={16} strokeWidth={2} /></button
                    >
                    <div class="markdown-content chat-message-text">
                      {@html renderMarkdown(tree.event.payload.text)}
                    </div>{/if}{#if tree.event.payload.attachments !== undefined && tree.event.payload.attachments.length > 0}<div
                      class="message-files"
                      aria-label="Attached files"
                    >
                      {#each tree.event.payload.attachments as file (file.id)}<a
                          class="message-file"
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
                {#if activityEvents(tree).length > 0 || controller.state.awaitingReplyFor.includes(tree.event.ref.id) || replyCanBeCancelled(tree)}<section
                    class="agent-activity-section"
                  >
                    <p class="agent-activity-heading">
                      {hasCancellationSuccess(tree)
                        ? "Cancelled"
                        : cancellationRequest(tree) !== undefined
                          ? "Cancellation requested"
                          : finalReplies(tree).length === 0
                            ? `${activityAgentLabel(tree)} is working`
                            : activityAgentLabel(
                                tree,
                              )}{#if finalReplies(tree).length > 0 && replyDuration(tree) !== ""}<span
                          class="agent-activity-duration"
                          >{replyDuration(tree)}</span
                        >{/if}{#if replyCanBeCancelled(tree) && workingReplyDuration(tree) !== ""}<span
                          class="agent-activity-duration"
                          >{workingReplyDuration(tree)}</span
                        >{/if}{#if replyCanBeCancelled(tree)}<button
                          class="agent-activity-cancel"
                          type="button"
                          disabled={controller.state.cancellingReplyFor.has(
                            tree.event.ref.id,
                          )}
                          onclick={() => void controller.cancelReply(tree)}
                          >{controller.state.cancellingReplyFor.has(
                            tree.event.ref.id,
                          )
                            ? "Cancelling..."
                            : "Cancel"}</button
                        >{/if}
                    </p>
                    {#if renderedActivityEvents(tree).length > 0}<div
                        class="agent-activity"
                      >
                        {#if renderedActivityEvents(tree).length > 5 && !controller.state.expandedActivity.has(tree.event.ref.id)}<p
                            class="agent-activity-overflow"
                          >
                            <span
                              >({renderedActivityEvents(tree).length - 5} more)</span
                            ><button
                              type="button"
                              onclick={() => controller.toggleActivity(tree)}
                              >Show all</button
                            >
                          </p>{/if}{#each displayedActivityEvents(tree, controller.state.expandedActivity) as event (event.event.ref.id)}{#if event.event.kind === "tool.request"}<p
                              class:tool-call-failed={toolStatus(event) ===
                                "failed"}
                              class:tool-call-succeeded={toolStatus(event) ===
                                "succeeded"}
                              class="tool-call"
                              title={event.event.payload.name ?? "tool"}
                            >
                              {#if toolStatus(event) === "working"}<span
                                  class="tool-status tool-status-working"
                                  aria-hidden="true"

                                ></span>{:else if toolStatus(event) === "succeeded"}<CircleCheck
                                  class="tool-status"
                                  size={14}
                                  strokeWidth={2}
                                  aria-hidden="true"
                                />{:else}<CircleX
                                  class="tool-status"
                                  size={14}
                                  strokeWidth={2}
                                  aria-hidden="true"
                                />{/if}Task: {event.event.payload.reason ??
                                `Running ${event.event.payload.name ?? "tool"}`}{#if activityDuration(event, "tool.success", "tool.failure") !== ""}<span
                                  class="tool-call-duration"
                                  >{activityDuration(
                                    event,
                                    "tool.success",
                                    "tool.failure",
                                  )}</span
                                >{/if}
                            </p>
                            {#each approvalRequests(event) as approval (approval.event.ref.id)}{@const response =
                                approvalResponse(approval)}
                              <section
                                class:approval-request-resolved={response !==
                                  undefined}
                                class="approval-request"
                              >
                                {#if response === undefined}<ShieldQuestionMark
                                    class="approval-request-icon"
                                    size={15}
                                    strokeWidth={2}
                                    aria-hidden="true"
                                  /><span class="approval-request-heading"
                                    >Action approval required:</span
                                  ><span class="approval-request-detail"
                                    >{approvalDescription(
                                      approval,
                                      event,
                                    )}</span
                                  ><span class="approval-request-actions"
                                    ><button
                                      class="approval-approve"
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
                                      class="approval-reject"
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
                                    class="approval-request-icon approval-request-approved"
                                    size={15}
                                    strokeWidth={2}
                                    aria-hidden="true"
                                  /><span class="approval-request-heading"
                                    >Action approved:</span
                                  ><span class="approval-request-detail"
                                    >{approvalDescription(
                                      approval,
                                      event,
                                    )}</span
                                  >{:else}<ShieldX
                                    class="approval-request-icon approval-request-rejected"
                                    size={15}
                                    strokeWidth={2}
                                    aria-hidden="true"
                                  /><span class="approval-request-heading"
                                    >Action rejected:</span
                                  ><span class="approval-request-detail"
                                    >{approvalDescription(
                                      approval,
                                      event,
                                    )}</span
                                  >{/if}
                              </section>
                              {#if response === undefined && (controller.state.approvalErrors.get(approval.event.ref.id) ?? "") !== ""}<p
                                  class="approval-request-error"
                                  role="alert"
                                >
                                  {controller.state.approvalErrors.get(
                                    approval.event.ref.id,
                                  )}
                                </p>{/if}{/each}{:else if event.event.kind === "thinking.started"}<p
                              class:tool-call-failed={thinkingStatus(event) ===
                                "failed"}
                              class:tool-call-succeeded={thinkingStatus(
                                event,
                              ) === "succeeded"}
                              class="tool-call"
                            >
                              {#if thinkingStatus(event) === "working"}<span
                                  class="tool-status tool-status-working"
                                  aria-hidden="true"

                                ></span>{:else if thinkingStatus(event) === "succeeded"}<CircleCheck
                                  class="tool-status"
                                  size={14}
                                  strokeWidth={2}
                                  aria-hidden="true"
                                />{:else}<CircleX
                                  class="tool-status"
                                  size={14}
                                  strokeWidth={2}
                                  aria-hidden="true"
                                />{/if}{thinkingStatus(event) === "working"
                                ? "Thinking"
                                : thinkingStatus(event) === "succeeded"
                                  ? "Thought"
                                  : "Thinking failed after"}{#if activityDuration(event, "thinking.completed", "thinking.failed") !== ""}<span
                                  class="tool-call-duration"
                                  >{activityDuration(
                                    event,
                                    "thinking.completed",
                                    "thinking.failed",
                                  )}</span
                                >{/if}
                            </p>{/if}{/each}{#if renderedActivityEvents(tree).length > 5 && controller.state.expandedActivity.has(tree.event.ref.id)}<p
                            class="agent-activity-overflow"
                          >
                            <span
                              >({renderedActivityEvents(tree).length} steps)</span
                            ><button
                              type="button"
                              onclick={() => controller.toggleActivity(tree)}
                              >Show less</button
                            >
                          </p>{/if}
                      </div>{/if}
                  </section>{/if}{#each finalReplies(tree) as reply (reply.event.ref.id)}<article
                    class="chat-message"
                  >
                    <p class="chat-message-author">
                      {reply.event.author_agent === undefined
                        ? "Gatehouse"
                        : agentLabel(reply.event.author_agent.id)}
                    </p>
                    {#if reply.event.payload.text !== ""}<button
                        class="chat-message-copy"
                        type="button"
                        aria-label="Copy response Markdown"
                        title="Copy Markdown"
                        onclick={() =>
                          void copyMarkdown(reply.event.payload.text ?? "")}
                        ><Copy size={16} strokeWidth={2} /></button
                      >
                      <div class="markdown-content chat-message-text">
                        {@html renderMarkdown(reply.event.payload.text ?? "")}
                      </div>{:else if reply.event.payload.attachments === undefined || reply.event.payload.attachments.length === 0}<div
                        class="chat-message-text"
                      >
                        <em>No reply.</em>
                      </div>{/if}{#if reply.event.payload.attachments !== undefined && reply.event.payload.attachments.length > 0}<div
                        class="message-files"
                        aria-label="Attached files"
                      >
                        {#each reply.event.payload.attachments as file (file.id)}<a
                            class="message-file"
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
                  </article>{/each}{/if}{/each}{/if}{#if controller.state.showJumpToLatest}<button
              class="button is-small chat-jump"
              type="button"
              onclick={() => void controller.jumpToLatest()}
              >Jump to latest</button
            >{/if}
      </div>
      {/if}
      </SidebarPage.Body>
      {#if session !== null}<SidebarPage.Footer placement="floating"><form
          class:sending={controller.state.sendingMessage}
          class="chat-composer"
          autocomplete="off"
          onsubmit={(event) => {
            event.preventDefault()
            void controller.sendMessage()
          }}
        >
          <label class="is-sr-only" for="message">Message</label><input
            class="is-sr-only"
            id="files"
            type="file"
            autocomplete="off"
            multiple
            bind:this={fileInputElement}
            onchange={(event) =>
              controller.selectComposerFiles(event.currentTarget)}
          />{#if controller.state.composerFiles.length > 0}<div
              class="composer-files"
              aria-label="Selected files"
            >
              {#each controller.state.composerFiles as entry (entry.file)}<span
                  class:failed={entry.status === "failed"}
                  class="composer-file"
                  >{#if entry.status === "uploading"}<span
                      class="composer-file-spinner"
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
                    type="button"
                    aria-label={`Remove ${entry.file.name}`}
                    disabled={controller.state.sendingMessage}
                    onclick={() => controller.removeComposerFile(entry.file)}
                    ><X size={14} strokeWidth={2} /></button
                  ></span
                >{/each}
            </div>{/if}
          <div class="chat-composer-row">
            <button
              class="chat-composer-attach"
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
            <div
              class:agent-selected={controller.state.selectedAgent !== ""}
              class="chat-composer-agent"
              title="Select agent"
            >
              <Bot size={20} strokeWidth={2.25} aria-hidden="true" /><select
                id="agent"
                aria-label="Agent"
                bind:value={controller.state.selectedAgent}
                ><option value="">Automatic</option
                >{#each controller.state.agents as agent}<option
                    value={agent.id}>{agent.label ?? agent.id}</option
                  >{/each}</select
              >
            </div>
            <textarea
              id="message"
              class="textarea"
              rows="1"
              autocomplete="off"
              placeholder="Write a message"
              bind:this={messageInputElement}
              bind:value={controller.state.messageText}
              disabled={controller.state.sendingMessage}
              oninput={(event) => resizeComposer(event.currentTarget)}
              onkeydown={(event) => {
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault()
                  void controller.sendMessage()
                }
              }}></textarea><button
              class="button is-primary chat-composer-send"
              type="submit"
              aria-label="Send message"
              title="Send message"
              disabled={controller.state.sendingMessage ||
                (controller.state.messageText.trim() === "" &&
                  controller.state.composerFiles.length === 0)}
              ><Send size={20} strokeWidth={2.25} aria-hidden="true" /></button
            >
          </div>
          {#if controller.state.messageError !== ""}<p
              class="help is-danger"
              aria-live="polite"
            >
              {controller.state.messageError}
            </p>{/if}
        </form></SidebarPage.Footer>{/if}
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
