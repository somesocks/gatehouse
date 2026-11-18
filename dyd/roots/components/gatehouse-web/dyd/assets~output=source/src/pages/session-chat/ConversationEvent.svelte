<script lang="ts">
  import * as UI from "../../ui"
  import { Paperclip, renderMarkdown } from "../../ui"
  import {
    chatFileDownloadPath,
    type ChatBranch,
    type ChatEvent,
  } from "../../app/chat"
  import { routePath } from "../../route"
  import { useRuntime } from "../../app/runtime.svelte"
  import { agentLabel } from "./agent-attribution"
  import { targetLabel } from "./message-targets"
  import type { createChatController } from "./chat-controller.svelte"
  import ConversationInteraction from "./ConversationInteraction.svelte"

  let {
    event,
    branch,
    controller,
    workspaceID,
    sessionID,
    onAuthenticationLost,
    anchor = false,
  }: {
    event: ChatEvent
    branch: ChatBranch
    controller: ReturnType<typeof createChatController>
    workspaceID: string
    sessionID: string
    onAuthenticationLost: () => void
    anchor?: boolean
  } = $props()
  const runtime = useRuntime()
  const request = $derived(
    !anchor && event.kind === "agent.request"
      ? branch.agent_requests.find((item) => item.request.id === event.ref.id)
      : undefined,
  )
  const displayed = $derived(request?.outcome ?? event)
  const interaction = $derived(
    anchor
      ? undefined
      : branch.interactions.find(
          (item) => item.request.ref.id === event.ref.id,
        ),
  )
  const ownedInteractions = $derived(
    new Set(request?.interactions.map((ref) => ref.id) ?? []),
  )
  const requestInteractions = $derived(
    branch.interactions.filter((item) =>
      ownedInteractions.has(item.request.ref.id),
    ),
  )
  const name = $derived(
    displayed.kind === "agent.request"
      ? agentLabel(displayed.target_agent)
      : displayed.author_agent
        ? agentLabel(displayed.author_agent)
        : (displayed.author_principal?.name ??
          displayed.author_principal?.alias ??
          (displayed.kind === "message.text" ? "User" : "Gatehouse")),
  )
  const avatarKind = $derived(
    displayed.author_agent !== undefined || displayed.kind.startsWith("agent.")
      ? "agent"
      : "person",
  )
  const statusLabel = $derived(
    request
      ? {
          working: "Working",
          waiting: "Waiting for input or approval",
          cancelling: "Cancelling",
          completed: "Completed",
          failed: "Failed",
          cancelled: "Cancelled",
        }[request.state]
      : "Status unavailable",
  )
  const canReply = $derived(
    !anchor &&
      (displayed.kind === "message.text" ||
        displayed.kind === "agent.success" ||
        displayed.kind === "agent.failure" ||
        event.kind === "agent.request"),
  )
  const failureText = $derived.by(() => {
    if (displayed.kind !== "agent.failure") return undefined
    const { code, message } = displayed.payload
    if (code === "cancelled") {
      const reason = message?.trim()
      return reason && reason.toLowerCase() !== "cancelled"
        ? `Cancelled: ${reason}`
        : "Cancelled"
    }
    return `Failed: ${message ?? code}`
  })
  const copyText = $derived(
    displayed.kind === "message.text" || displayed.kind === "agent.success"
      ? displayed.payload.text
      : failureText,
  )
  const repliesPath = (id: string) =>
    routePath({ kind: "session-chat", workspaceID, sessionID, parentID: id })
</script>

{#snippet messageActions()}
  <UI.MessageActions
    text={copyText}
    onreply={canReply
      ? () => runtime.navigate(repliesPath(displayed.ref.id))
      : undefined}
  />
{/snippet}

{#if anchor || displayed.kind === "message.text" || displayed.kind === "agent.success" || displayed.kind === "agent.failure" || event.kind === "agent.request" || interaction !== undefined}
  <UI.Message
    timestamp={displayed.created_at}
    purpose={displayed.kind === "agent.success" ? "output" : undefined}
    actions={copyText || canReply ? messageActions : undefined}
  >
    {#snippet avatar()}<UI.Avatar
        kind={avatarKind}
        label={name}
        aria-hidden="true"
      />{/snippet}
    {#snippet header()}{name}{/snippet}
    {#snippet metadata()}
      {#if displayed.kind === "message.text" && (displayed.payload.targets?.length ?? 0) > 0}
        <UI.Text aria-label="Message recipients"
          >To: {displayed.payload.targets?.map(targetLabel).join(", ")}</UI.Text
        >
      {/if}
    {/snippet}
    {#if requestInteractions.length > 0}
      <UI.Stack
        as="section"
        gap="small"
        aria-label="Agent input and approval activity"
      >
        {#each requestInteractions as interaction (interaction.request.ref.id)}
          <ConversationInteraction
            {interaction}
            {controller}
            {workspaceID}
            {sessionID}
            {onAuthenticationLost}
          />
        {/each}
      </UI.Stack>
    {/if}
    {#if displayed.kind === "message.text" || displayed.kind === "agent.success"}
      {#if displayed.payload.text}<UI.RichText
          html={renderMarkdown(displayed.payload.text)}
        />
      {:else if displayed.kind === "agent.success" && !displayed.payload.attachments?.length}<UI.Text
          as="em"
          size="small"
          tone="muted">No reply.</UI.Text
        >{/if}
      {#if displayed.payload.attachments?.length}
        <UI.Row aria-label="Attached files">
          {#each displayed.payload.attachments as file (file.id)}
            <UI.Chip
              href={chatFileDownloadPath(workspaceID, sessionID, file.id)}
              target="_blank"
              rel="noopener noreferrer"
              download={file.name}
              title={file.fingerprint}
            >
              <Paperclip size={14} strokeWidth={2} aria-hidden="true" />
              <UI.Text>{file.name}</UI.Text><UI.Text as="small" size="small"
                >{file.size} bytes{file.media_type
                  ? ` · ${file.media_type}`
                  : ""}</UI.Text
              >
            </UI.Chip>
          {/each}
        </UI.Row>
      {/if}
    {:else if displayed.kind === "agent.failure"}
      <UI.Row tone="muted" role="status" aria-label={`${name} status`}>
        <UI.Text>{failureText}</UI.Text>
      </UI.Row>
    {:else if anchor}
      <UI.Text>
        {#if displayed.kind === "input.request" || displayed.kind === "approval.request"}
          {displayed.payload.description}
        {:else if displayed.kind === "agent.request"}
          Agent reply request.
        {:else}
          This event has no message text.
        {/if}
      </UI.Text>
    {:else if event.kind === "agent.request"}
      <UI.Row tone="muted" role="status" aria-label={`${name} status`}>
        {#if request?.state === "working" || request?.state === "cancelling"}<UI.Spinner
            aria-hidden="true"
          />{/if}
        <UI.Text>{statusLabel}</UI.Text>
        {#if request?.state === "working" || request?.state === "waiting"}
          <UI.InlineButton
            type="button"
            size="default"
            disabled={controller.state.cancellingReplyFor.has(event.ref.id)}
            onclick={() => void controller.cancelReply({ event, children: [] })}
            >{controller.state.cancellingReplyFor.has(event.ref.id)
              ? "Cancelling..."
              : "Cancel"}</UI.InlineButton
          >
        {/if}
      </UI.Row>
    {:else if interaction}
      <ConversationInteraction
        {interaction}
        {controller}
        {workspaceID}
        {sessionID}
        {onAuthenticationLost}
      />
    {/if}
  </UI.Message>
{/if}
