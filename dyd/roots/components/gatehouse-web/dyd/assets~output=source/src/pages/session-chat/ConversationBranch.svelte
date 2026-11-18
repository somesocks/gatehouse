<script lang="ts">
  import * as UI from "../../ui"
  import type { ChatEvent } from "../../app/chat"
  import type { createChatController } from "./chat-controller.svelte"
  import ConversationEvent from "./ConversationEvent.svelte"
  import { routePath } from "../../route"

  let {
    controller,
    workspaceID,
    sessionID,
    onAuthenticationLost,
  }: {
    controller: ReturnType<typeof createChatController>
    workspaceID: string
    sessionID: string
    onAuthenticationLost: () => void
  } = $props()
  const branch = $derived(controller.state.branch)
  const path = (parentID: string) =>
    routePath({ kind: "session-chat", workspaceID, sessionID, parentID })
</script>

{#if controller.state.sentMessageID && !branch.entries.some((entry) => entry.event.ref.id === controller.state.sentMessageID)}
  <UI.Notice role="status"
    >Message sent. <UI.RouterLink href={path(controller.state.sentMessageID)}
      >View its replies</UI.RouterLink
    ></UI.Notice
  >
{/if}
{#if branch.entries.length === 0}
  <UI.Text as="p" tone="muted"
    >{controller.state.parentID
      ? "No replies in this branch yet."
      : "Send the first message to begin."}</UI.Text
  >
{/if}
{#snippet conversationEvent(event: ChatEvent, allowReply: boolean)}
  <ConversationEvent
    {event}
    {allowReply}
    {branch}
    {controller}
    {workspaceID}
    {sessionID}
    {onAuthenticationLost}
  />
  {#if event.kind === "agent.request"}
    {@const request = branch.agent_requests.find(
      (item) => item.request.id === event.ref.id,
    )}
    {#if request?.outcome && (request.outcome_previews?.length || request.outcome_has_more_replies)}
      <UI.MessageReplies aria-label="Replies to agent message">
        {#each request.outcome_previews ?? [] as reply (reply.ref.id)}
          <ConversationEvent
            event={reply}
            allowReply={false}
            {branch}
            {controller}
            {workspaceID}
            {sessionID}
            {onAuthenticationLost}
          />
        {/each}
        <UI.RouterLink href={path(request.outcome.ref.id)}
          >See all replies</UI.RouterLink
        >
      </UI.MessageReplies>
    {/if}
  {/if}
{/snippet}

{#each branch.entries as entry (entry.event.ref.id)}
  <UI.Stack as="section" gap="small" aria-label="Conversation entry">
    {@render conversationEvent(entry.event, true)}
    {#if entry.previews.length > 0 || entry.has_more_replies}
      <UI.MessageReplies aria-label="Reply previews">
        {#each entry.previews as event (event.ref.id)}
          {@render conversationEvent(event, false)}
        {/each}
        {#if entry.has_more_replies}
          <UI.RouterLink href={path(entry.event.ref.id)}
            >See all replies</UI.RouterLink
          >
        {/if}
      </UI.MessageReplies>
    {/if}
  </UI.Stack>
{/each}
{#if branch.next_cursor}
  <UI.Button
    type="button"
    variant="outline"
    disabled={controller.state.reloading}
    onclick={() => void controller.loadMore()}
    >{controller.state.reloading
      ? "Loading..."
      : "Load more messages"}</UI.Button
  >
{/if}
