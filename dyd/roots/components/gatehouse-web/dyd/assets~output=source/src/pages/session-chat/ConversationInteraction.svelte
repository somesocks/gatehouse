<script lang="ts">
  import * as UI from "../../ui"
  import type { ChatBranch, ChatEventTree } from "../../app/chat"
  import type { createChatController } from "./chat-controller.svelte"
  import InputRequestCard from "./InputRequestCard.svelte"
  import InputRequestActivity from "./InputRequestActivity.svelte"

  let {
    interaction,
    controller,
    workspaceID,
    sessionID,
    onAuthenticationLost,
  }: {
    interaction: ChatBranch["interactions"][number]
    controller: ReturnType<typeof createChatController>
    workspaceID: string
    sessionID: string
    onAuthenticationLost: () => void
  } = $props()
  const event = $derived(interaction.request)
  const response = $derived(interaction.response)
  // The scoped input components consume a request and its actual resolution;
  // neither form descriptors nor drafts come from the branch projection.
  const input = $derived(
    event.kind === "input.request"
      ? ({
          event,
          children: response ? [{ event: response, children: [] }] : [],
        } satisfies ChatEventTree<"input.request">)
      : undefined,
  )
</script>

{#if input}
  <InputRequestActivity request={input} />
  <InputRequestCard
    request={input}
    {workspaceID}
    {sessionID}
    {onAuthenticationLost}
    onResolved={() => void controller.refreshEvents()}
  />
{:else if event.kind === "approval.request"}
  <UI.Row tone="muted" data-resolved={response !== undefined || undefined}>
    <UI.Text as="strong" emphasis="strong"
      >{response?.kind === "approval.success"
        ? "Action approved:"
        : response?.kind === "approval.failure"
          ? response.payload.code === "cancelled"
            ? "Action cancelled:"
            : "Action rejected:"
          : "Action approval required:"}</UI.Text
    >
    <UI.Text>{event.payload.description ?? "Approve this action"}</UI.Text>
    {#if response === undefined}
      <UI.Button
        type="button"
        size="sm"
        disabled={controller.state.submittingApprovals.has(event.ref.id)}
        onclick={() =>
          void controller.respondToApproval(
            { event, children: [] },
            "approved",
          )}
        >{controller.state.submittingApprovals.has(event.ref.id)
          ? "Submitting..."
          : "Approve"}</UI.Button
      >
      <UI.Button
        type="button"
        size="sm"
        variant="outline"
        disabled={controller.state.submittingApprovals.has(event.ref.id)}
        onclick={() =>
          void controller.respondToApproval(
            { event, children: [] },
            "rejected",
          )}>Reject</UI.Button
      >
    {/if}
  </UI.Row>
  {#if response === undefined && controller.state.approvalErrors.get(event.ref.id)}
    <UI.Text as="p" role="alert"
      >{controller.state.approvalErrors.get(event.ref.id)}</UI.Text
    >
  {/if}
{/if}
