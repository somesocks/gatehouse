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
  import {
    chatFileDownloadPath,
    fetchChatSession,
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

  $effect(() => {
    const route = currentRoute
    return activate(route)
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
                  {#if activityEvents(tree).length > 0 || controller.state.awaitingReplyFor.includes(tree.event.ref.id) || replyCanBeCancelled(tree)}<section
                      class="event-log"
                    >
                      <header>
                        {hasCancellationSuccess(tree)
                          ? "Cancelled"
                          : cancellationRequest(tree) !== undefined
                            ? "Cancellation requested"
                            : finalReplies(tree).length === 0
                              ? `${activityAgentLabel(tree)} is working`
                              : activityAgentLabel(
                                  tree,
                                )}{#if finalReplies(tree).length > 0 && replyDuration(tree) !== ""}<small>{replyDuration(tree)}</small>{/if}{#if replyCanBeCancelled(tree) && workingReplyDuration(tree) !== ""}<small>{workingReplyDuration(tree)}</small>{/if}{#if replyCanBeCancelled(tree)}<button
                            class="inline"
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
                      </header>
                      {#if renderedActivityEvents(tree).length > 0}<div
                          class="event-list"
                        >
                          {#if renderedActivityEvents(tree).length > 5 && !controller.state.expandedActivity.has(tree.event.ref.id)}<p
                              class="event-more"
                            >
                              <span
                                >({renderedActivityEvents(tree).length - 5} more)</span
                               ><button
                                  class="primary inline"
                                  type="button"
                                onclick={() => controller.toggleActivity(tree)}
                                >Show all</button
                              >
                            </p>{/if}{#each displayedActivityEvents(tree, controller.state.expandedActivity) as event (event.event.ref.id)}{#if event.event.kind === "tool.request"}<p
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
                                  />{/if}{thinkingStatus(event) === "working"
                                  ? "Thinking"
                                  : thinkingStatus(event) === "succeeded"
                                    ? "Thought"
                                    : "Thinking failed after"}{#if activityDuration(event, "thinking.completed", "thinking.failed") !== ""}<small>{activityDuration(
                                      event,
                                      "thinking.completed",
                                      "thinking.failed",
                                    )}</small>{/if}
                              </p>{/if}{/each}{#if renderedActivityEvents(tree).length > 5 && controller.state.expandedActivity.has(tree.event.ref.id)}<p
                              class="event-more"
                            >
                              <span
                                >({renderedActivityEvents(tree).length} steps)</span
                               ><button
                                  class="primary inline"
                                  type="button"
                                onclick={() => controller.toggleActivity(tree)}
                                >Show less</button
                              >
                            </p>{/if}
                        </div>{/if}
                    </section>{/if}{#each finalReplies(tree) as reply (reply.event.ref.id)}<article
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
                    </article>{/each}{/if}{/each}{/if}{#if controller.state.showJumpToLatest}<button
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
              void controller.sendMessage()
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
              <div
                data-select
                data-selected={controller.state.selectedAgent !== "" || undefined}
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
                rows="1"
                autocomplete="off"
                placeholder="Write a message"
                bind:this={messageInputElement}
                bind:value={controller.state.messageText}
                disabled={controller.state.sendingMessage}
                onkeydown={(event) => {
                  if (event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault()
                    void controller.sendMessage()
                  }
                }}></textarea><button
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
          </form></SidebarPage.Footer
        >{/if}
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
