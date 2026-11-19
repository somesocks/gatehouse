<script lang="ts">
  import {
    ArrowDown,
    Send,
    AtSign,
    Slash,
    Paperclip,
    RouterLink,
    StatusPage,
  } from "../../ui"
  import * as UI from "../../ui"
  import * as SidebarPage from "../../ui/sidebar-page"
  import {
    workspaceNavigation,
    sessionViews,
    renameSession,
  } from "../../app/navigation"
  import SessionViewNavigation from "../../session-participants/SessionViewNavigation.svelte"
  import { onMount, tick, untrack } from "svelte"
  import {
    createAsyncResource,
    createLoadState,
  } from "../../utils/load-state.svelte"
  import { fetchChatSession, type MessageTargetRef } from "../../app/chat"
  import { fetchWorkspaceGroupMemberships, type Group } from "../../app/groups"
  import { useRuntime } from "../../app/runtime.svelte"
  import { routePath, type Route } from "../../route"
  import RecipientPicker from "./RecipientPicker.svelte"
  import ChatCommandPicker from "./ChatCommandPicker.svelte"
  import SendAsPicker from "./SendAsPicker.svelte"
  import ConversationBranch from "./ConversationBranch.svelte"
  import ConversationEvent from "./ConversationEvent.svelte"
  import { createChatController } from "./chat-controller.svelte"
  import { createRecipientController } from "./recipient-controller.svelte"
  import { targetKey, targetLabel } from "./message-targets"
  import {
    insertedMentionStart,
    shouldSubmitComposerKey,
  } from "./composer-input"

  type SessionRoute = Extract<Route, { kind: "session-chat" }>
  type SessionTarget = Pick<SessionRoute, "workspaceID" | "sessionID">
  type Session = {
    id: string
    created_at: string
    name?: string
    project?: { id: string; name?: string }
  }
  let { route }: { route?: SessionRoute } = $props()
  const runtime = useRuntime()
  const { access, activity, auth } = runtime
  const sessionLoad = createLoadState()
  let session = $state<Session | null>(null)
  const sessionStatus = $derived(sessionLoad.status)
  let feedElement = $state<HTMLElement | undefined>()
  let messageInputElement = $state<HTMLTextAreaElement | undefined>()
  let fileInputElement = $state<HTMLInputElement | undefined>()
  let mentionOpen = $state(false)
  let mentionStart = -1
  let commandsOpen = $state(false)
  let slashTrigger: { index: number; draft: string } | undefined
  let sendAsOpen = $state(false)
  let openingSendAs = $state(false)
  let sendAsCursor: number | undefined
  let sendAsID = $state("")
  let sendAsStorageKey = ""
  const senderGroups = createAsyncResource<Group[]>()
  const sendAsGroup = $derived(
    senderGroups.value?.find((group) => group.id === sendAsID),
  )
  const sendAsUnavailable = $derived(
    sendAsID !== "" && senderGroups.lifecycle.state.hasLoaded && !sendAsGroup,
  )
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
  const rootPath = $derived(
    routePath({
      kind: "session-chat",
      workspaceID: currentRoute.workspaceID,
      sessionID: currentRoute.sessionID,
    }),
  )
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
    return (
      feedElement === undefined ||
      feedElement.scrollHeight -
        feedElement.scrollTop -
        feedElement.clientHeight <
        64
    )
  }
  async function followLatest(
    behavior: ScrollBehavior = "smooth",
  ): Promise<void> {
    const value = generation
    const parentID = currentRoute.parentID
    await tick()
    if (
      value !== generation ||
      abortController?.signal.aborted ||
      currentRoute.parentID !== parentID
    )
      return
    if (
      document.activeElement instanceof Element &&
      document.activeElement.closest("[data-input-form]") !== null
    )
      return
    feedElement?.scrollTo({ top: feedElement.scrollHeight, behavior })
  }
  const controller = untrack(() =>
    createChatController({
      isNearBottom,
      followLatest,
      focusComposer: () => {
        if (
          runtime.state.route.kind === "session-chat" &&
          !mentionOpen &&
          document.activeElement === messageInputElement
        )
          messageInputElement?.focus({ preventScroll: true })
      },
    }),
  )
  const recipients = createRecipientController()
  const sendLabel = $derived(
    !controller.state.sendingMessage
      ? currentRoute.parentID
        ? "Send reply"
        : "Send message"
      : controller.state.composerFiles.some(
            (entry) => entry.status === "uploading",
          )
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
    const parentID = currentRoute.parentID ?? ""
    untrack(() => {
      closeComposerMenus()
      controller.selectBranch(parentID)
    })
  })
  $effect(() => {
    if (controller.state.authenticationRequired) runtime.requireLogin()
  })
  onMount(() => {
    const copyCodeBlock = (event: MouseEvent) => {
      if (!(event.target instanceof Element)) return
      const code = event.target
        .closest<HTMLButtonElement>("[data-copy-code]")
        ?.parentElement?.querySelector("pre > code")
      if (code) void navigator.clipboard.writeText(code.textContent ?? "")
    }
    const refreshIdentities = () => {
      void recipients.refresh()
      if (sendAsID || sendAsOpen) void refreshSenderGroups()
    }
    document.addEventListener("click", copyCodeBlock)
    window.addEventListener("focus", refreshIdentities)
    return () => {
      document.removeEventListener("click", copyCodeBlock)
      window.removeEventListener("focus", refreshIdentities)
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
    closeComposerMenus()
    senderGroups.context("")
    sendAsID = ""
    sendAsStorageKey = ""
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
        senderGroups.stop()
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
      controller.start(workspaceID, sessionID, signal, currentRoute.parentID)
      const principalID = auth.state.claims?.principal.ref.id
      if (principalID) {
        recipients.start(principalID, workspaceID, sessionID, signal)
        sendAsStorageKey = `gatehouse:message-sender-group:${JSON.stringify([principalID, workspaceID, sessionID])}`
        senderGroups.context(sendAsStorageKey)
        try {
          sendAsID = localStorage.getItem(sendAsStorageKey) ?? ""
        } catch {
          // Storage may be unavailable.
        }
        if (sendAsID) void refreshSenderGroups()
      }
    } else await Promise.all([controller.refreshEvents(), recipients.refresh()])
    if (isCurrent(value, workspaceID, sessionID, signal))
      subscribe(route, value, signal)
  }
  async function loadSession(
    value: number,
    workspaceID: string,
    sessionID: string,
    signal: AbortSignal,
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
        request.fail("Chat information could not be loaded.")
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
          events: [
            "workspace_agent.*",
            "workspace_grant.*",
            "group.*",
            "group_member.*",
            "group_agent.*",
          ],
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
        if (names.has("session-chat-agent") && (sendAsID || sendAsOpen))
          void refreshSenderGroups(signal)
        const refreshed = await Promise.all([
          ...(names.has("session-chat-agent") || names.has("session-recipients")
            ? [recipients.refresh(signal)]
            : []),
          ...(names.has("session-chat") || names.has("session-chat-agent")
            ? [
                controller.refreshEvents(signal),
                loadSession(
                  value,
                  workspaceID,
                  sessionID,
                  AbortSignal.any([routeSignal, signal]),
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
  function closeComposerMenus(): void {
    mentionOpen = false
    mentionStart = -1
    commandsOpen = false
    slashTrigger = undefined
    sendAsOpen = false
    openingSendAs = false
    sendAsCursor = undefined
  }
  function openComposerMenuAfterInput(event: Event, data: string | null): void {
    if (
      mentionOpen ||
      commandsOpen ||
      sendAsOpen ||
      openingSendAs ||
      controller.state.sendingMessage ||
      !(event.currentTarget instanceof HTMLTextAreaElement)
    )
      return
    const input = event.currentTarget
    if (data === "/" && input.value[input.selectionStart - 1] === "/") {
      slashTrigger = { index: input.selectionStart - 1, draft: input.value }
      commandsOpen = true
      return
    }
    const start = insertedMentionStart(input.value, input.selectionStart, data)
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
    if (
      mentionStart >= 0 &&
      controller.state.messageText[mentionStart] === "@"
    ) {
      const text = controller.state.messageText
      controller.state.messageText =
        text.slice(0, mentionStart) + text.slice(mentionStart + 1)
    }
    mentionOpen = false
  }
  function openRecipients(): void {
    mentionStart = -1
    mentionOpen = true
    void recipients.refresh()
  }
  function refreshSenderGroups(
    signal = abortController?.signal,
  ): Promise<boolean> {
    const workspaceID = currentRoute.workspaceID
    return senderGroups.load(
      (signal) => fetchWorkspaceGroupMemberships(workspaceID, signal),
      signal,
    )
  }
  function openSendAs(): void {
    sendAsOpen = true
    void refreshSenderGroups()
  }
  function selectSendAsCommand(): void {
    const trigger = slashTrigger
    if (trigger && controller.state.messageText === trigger.draft) {
      const { index, draft } = trigger
      controller.state.messageText =
        draft.slice(0, index) + draft.slice(index + 1)
      sendAsCursor = index
    }
    openingSendAs = true
    commandsOpen = false
  }
  function closeCommands(): void {
    slashTrigger = undefined
    if (openingSendAs) {
      openingSendAs = false
      openSendAs()
    } else if (!sendAsOpen) focusComposer()
  }
  function selectSenderGroup(id: string): void {
    sendAsID = id
    if (!sendAsStorageKey) return
    try {
      if (id) localStorage.setItem(sendAsStorageKey, id)
      else localStorage.removeItem(sendAsStorageKey)
    } catch {
      // Keep the in-memory selection if storage is unavailable.
    }
  }
  async function sendMessage(): Promise<void> {
    if (!recipients.canSend || sendAsUnavailable) return
    await controller.sendMessage(recipients.state.targets, sendAsID)
    void recipients.refresh()
  }
</script>

{#snippet threadHeader()}
  <UI.Stack gap="normal">
    <UI.Stack gap="none">
      <UI.Row as="nav" aria-label="Branch navigation">
        <UI.RouterLink href={rootPath}>
          <UI.Row as="span" gap="small">
            <UI.ArrowLeft size={14} strokeWidth={2} aria-hidden="true" /> Back
          </UI.Row>
        </UI.RouterLink>
      </UI.Row>
      <UI.Heading level={2} size="small">Replying to</UI.Heading>
    </UI.Stack>
    {#if controller.state.branch.parent && controller.state.branch.parent.ref.id === currentRoute.parentID && controller.state.parentID === currentRoute.parentID}
      <UI.Stack as="section" aria-label="Thread anchor">
        <ConversationEvent
          event={controller.state.branch.parent}
          branch={controller.state.branch}
          {controller}
          workspaceID={currentRoute.workspaceID}
          sessionID={currentRoute.sessionID}
          onAuthenticationLost={runtime.requireLogin}
          anchor
        />
      </UI.Stack>
    {/if}
  </UI.Stack>
{/snippet}

{#if auth.state.status === "checking" || (auth.state.status === "authenticated" && access.state.workspaceStatus === "checking")}
  <StatusPage eyebrow="Gatehouse" busy live="polite"
    ><UI.Text as="p"
      >{auth.state.status === "checking"
        ? "Checking your session."
        : "Loading your workspaces."}</UI.Text
    ></StatusPage
  >
{:else if auth.state.status === "unavailable" || access.state.workspaceStatus === "unavailable"}
  <StatusPage
    eyebrow="Gatehouse"
    title="Connection unavailable"
    description="Gatehouse could not load your account."
    ><UI.Button type="button" onclick={() => void runtime.refresh()}
      >Try again</UI.Button
    ></StatusPage
  >
{:else if auth.state.status !== "authenticated"}
  <StatusPage eyebrow="Gatehouse" title="Sign in required"
    ><UI.RouterLink href={runtime.loginHref} replace>Sign in</UI.RouterLink
    ></StatusPage
  >
{:else if access.state.workspaceStatus === "empty" || workspace === null}
  <StatusPage
    eyebrow="Gatehouse"
    title="No workspace access"
    description="Ask an administrator to add you to a workspace group."
    ><UI.RouterLink href="/app/no-access" replace>Continue</UI.RouterLink
    ></StatusPage
  >
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
        <UI.Breadcrumb.Root aria-label="Breadcrumb" data-page-breadcrumb
          ><UI.Breadcrumb.List>
            <UI.Breadcrumb.Item
              ><RouterLink href={workspacePath(workspace.id)}
                >{workspace.name ?? "Workspace"}</RouterLink
              ></UI.Breadcrumb.Item
            >
            {#if session !== null}
              {#if session.project !== undefined}<UI.Breadcrumb.Separator
                /><UI.Breadcrumb.Item
                  ><RouterLink
                    href={projectPath(workspace.id, session.project.id)}
                    >{session.project.name ?? "New Project"}</RouterLink
                  ></UI.Breadcrumb.Item
                >{/if}
              <UI.Breadcrumb.Separator /><UI.Breadcrumb.Item aria-current="page"
                ><UI.Breadcrumb.Page
                  >{session.name ?? "New Chat"}</UI.Breadcrumb.Page
                ></UI.Breadcrumb.Item
              >
            {/if}
          </UI.Breadcrumb.List></UI.Breadcrumb.Root
        >
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
        <UI.Feed
          header={currentRoute.parentID ? threadHeader : undefined}
          bind:element={feedElement}
          aria-live="polite"
          aria-busy={sessionStatus === "checking" ||
            controller.state.status === "checking"}
          onscroll={() => controller.trackScroll()}
        >
          {#if sessionLoad.state.error || controller.state.reloadError}
            <UI.Notice variant="destructive" role="alert"
              >{sessionLoad.state.error ||
                controller.state.reloadError}<UI.Button
                variant="outline"
                type="button"
                onclick={() =>
                  void loadRoute(
                    currentRoute,
                    generation,
                    abortController!.signal,
                  )}>Retry</UI.Button
              ></UI.Notice
            >
          {/if}
          {#if sessionStatus === "checking" || (sessionStatus === "ready" && controller.state.status === "checking")}
            <UI.Text as="p" tone="muted"
              ><UI.Spinner aria-hidden="true" /> Loading chat...</UI.Text
            >
          {:else if sessionStatus === "ready" && session !== null && controller.state.status === "ready"}
            <ConversationBranch
              {controller}
              workspaceID={workspace.id}
              sessionID={session.id}
              onAuthenticationLost={runtime.requireLogin}
            />
          {/if}
          {#snippet actions()}{#if controller.state.showJumpToLatest}<UI.FloatingAction
                variant="default"
                size="icon-lg"
                type="button"
                aria-label="Jump to bottom"
                title="Jump to bottom"
                onclick={() => void controller.jumpToLatest()}
                ><ArrowDown
                  size={20}
                  strokeWidth={2.25}
                  aria-hidden="true"
                /></UI.FloatingAction
              >{/if}{/snippet}
        </UI.Feed>
      </SidebarPage.Body>
      {#if session !== null}
        <SidebarPage.Footer compact
          ><UI.InputBar
            hasText={controller.state.messageText.length > 0}
            overlayOpen={mentionOpen ||
              commandsOpen ||
              openingSendAs ||
              sendAsOpen}
            data-pending={controller.state.sendingMessage || undefined}
            aria-busy={controller.state.sendingMessage}
            autocomplete="off"
            onsubmit={(event) => {
              event.preventDefault()
              void sendMessage()
            }}
          >
            {#if recipients.state.targets.length > 0 || sendAsID}
              <UI.InputBarAttachments label="Message addressing">
                {#if recipients.state.targets.length > 0}
                  <UI.InlineButton
                    type="button"
                    disabled={controller.state.sendingMessage}
                    onclick={openRecipients}>To:</UI.InlineButton
                  >
                  {#each recipients.state.targets as ref (targetKey(ref))}
                    <UI.InputRecipient
                      name={targetLabel(recipients.target(ref))}
                      unavailable={recipients.state.ready &&
                        !recipients.available(ref)}
                      pending={recipients.state.ready &&
                        recipients.available(ref) &&
                        !recipients.inChat(ref)}
                      disabled={controller.state.sendingMessage}
                      onselect={openRecipients}
                      onremove={() =>
                        recipients.select(
                          recipients.state.targets.filter(
                            (target) => targetKey(target) !== targetKey(ref),
                          ),
                        )}
                    />
                  {/each}
                {/if}
                {#if sendAsID}
                  <UI.Row role="group" aria-label="Message sender" wrap={false}>
                    <UI.InlineButton
                      type="button"
                      disabled={controller.state.sendingMessage}
                      onclick={openSendAs}>Send as:</UI.InlineButton
                    >
                    <UI.InputRecipient
                      name={sendAsGroup?.name ?? sendAsGroup?.alias ?? sendAsID}
                      unavailable={sendAsUnavailable}
                      disabled={controller.state.sendingMessage}
                      onselect={openSendAs}
                      onremove={() => selectSenderGroup("")}
                    />
                  </UI.Row>
                {/if}
              </UI.InputBarAttachments>
            {/if}
            {#if recipients.state.error || (recipients.state.outsideError && recipients.state.targets.some((ref) => !recipients.inChat(ref)))}<UI.Row
                ><UI.Text as="p" size="small" role="alert"
                  >{recipients.state.error ||
                    recipients.state.outsideError}</UI.Text
                ><UI.Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onclick={() => void recipients.refresh()}>Try again</UI.Button
                ></UI.Row
              >{/if}
            <UI.Label hidden for="message">Message</UI.Label>
            <UI.FileInput
              hidden
              id="files"
              type="file"
              autocomplete="off"
              multiple
              bind:ref={fileInputElement}
              onchange={(event) =>
                controller.selectComposerFiles(event.currentTarget)}
            />
            {#if controller.state.composerFiles.length > 0}<UI.InputBarAttachments
                >{#each controller.state.composerFiles as entry (entry.file)}<UI.InputAttachment
                    name={entry.file.name}
                    size={entry.file.size}
                    status={entry.status}
                    uploaded={entry.id !== undefined}
                    error={entry.error}
                    disabled={controller.state.sendingMessage}
                    onremove={() => controller.removeComposerFile(entry.file)}
                  />{/each}</UI.InputBarAttachments
              >{/if}
            {#snippet input()}<UI.Textarea
                mode="composer"
                id="message"
                rows={1}
                enterkeyhint="enter"
                autocomplete="off"
                placeholder={currentRoute.parentID
                  ? "Write a reply"
                  : "Write a message"}
                aria-describedby={controller.state.messageError
                  ? "message-error"
                  : undefined}
                bind:ref={messageInputElement}
                bind:value={controller.state.messageText}
                onkeydown={(event) => {
                  const mobile = window.matchMedia(
                    "(max-width: 767px), (pointer: coarse)",
                  ).matches
                  if (shouldSubmitComposerKey(event, mobile)) {
                    event.preventDefault()
                    void sendMessage()
                  }
                }}
                oninput={(event) => {
                  if (
                    event instanceof InputEvent &&
                    event.inputType === "insertText" &&
                    !event.isComposing
                  )
                    openComposerMenuAfterInput(event, event.data)
                }}
                oncompositionend={(event) =>
                  openComposerMenuAfterInput(event, event.data)}
              />{/snippet}
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
                onclick={openRecipients}
                ><AtSign
                  size={20}
                  strokeWidth={2.25}
                  aria-hidden="true"
                /></UI.Button
              >
              <UI.Button
                variant="ghost"
                size="icon"
                type="button"
                aria-haspopup="dialog"
                aria-expanded={commandsOpen}
                aria-label="Chat commands"
                title="Chat commands"
                disabled={controller.state.sendingMessage}
                onclick={() => {
                  slashTrigger = undefined
                  commandsOpen = true
                }}
              >
                <Slash size={20} strokeWidth={2.25} aria-hidden="true" />
              </UI.Button>
            {/snippet}
            {#snippet trailing()}<UI.Button
                variant="default"
                size="icon"
                type="submit"
                aria-label={sendLabel}
                title={sendLabel}
                disabled={controller.state.sendingMessage ||
                  !controller.state.canCompose ||
                  !recipients.canSend ||
                  sendAsUnavailable ||
                  (controller.state.messageText.trim() === "" &&
                    controller.state.composerFiles.length === 0)}
                >{#if controller.state.sendingMessage}<UI.Spinner
                    aria-hidden="true"
                  />{:else}<Send
                    size={20}
                    strokeWidth={2.25}
                    aria-hidden="true"
                  />{/if}</UI.Button
              >{/snippet}
            {#snippet feedback()}{#if controller.state.messageError}<UI.Text
                  id="message-error"
                  as="p"
                  size="small"
                  role="alert"
                  aria-live="polite">{controller.state.messageError}</UI.Text
                >{/if}{/snippet}
            <ChatCommandPicker
              bind:open={commandsOpen}
              onSendAs={selectSendAsCommand}
              onclose={closeCommands}
            />
            <SendAsPicker
              bind:open={sendAsOpen}
              groups={senderGroups.value ?? []}
              selected={sendAsID}
              loading={senderGroups.lifecycle.loading ||
                senderGroups.lifecycle.reloading}
              error={senderGroups.lifecycle.state.error}
              onselect={selectSenderGroup}
              onretry={() => void refreshSenderGroups()}
              onclose={() => {
                focusComposer(sendAsCursor)
                sendAsCursor = undefined
              }}
            />
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
        >
      {/if}
    </SidebarPage.Page>
  </SidebarPage.Root>
{/if}
