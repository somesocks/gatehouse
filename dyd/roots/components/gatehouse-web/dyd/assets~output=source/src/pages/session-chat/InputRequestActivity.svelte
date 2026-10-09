<script lang="ts">
  import { CircleCheck, CircleX, ClipboardList } from "../../ui"
  import * as UI from "../../ui"
  import type { ChatEventTree } from "../../app/chat"
  import { inputResponse } from "./chat-controller.svelte"

  let { request }: { request: ChatEventTree<"input.request"> } = $props()
  const response = $derived(inputResponse(request))
</script>

<UI.Row tone="muted" data-resolved={response !== undefined || undefined}>
  {#if response?.event.kind === "input.success"}
    <CircleCheck size={15} strokeWidth={2} aria-hidden="true" />
    <UI.Text as="strong" emphasis="strong">Input submitted:</UI.Text>
  {:else if response?.event.kind === "input.failure"}
    <CircleX size={15} strokeWidth={2} aria-hidden="true" />
    <UI.Text as="strong" emphasis="strong"
      >{response.event.payload.code === "cancelled"
        ? "Input cancelled:"
        : "Input failed:"}</UI.Text
    >
  {:else}
    <ClipboardList size={15} strokeWidth={2} aria-hidden="true" />
    <UI.Text as="strong" emphasis="strong">Input requested:</UI.Text>
  {/if}
  <UI.Text>{request.event.payload.description ?? "Provide details"}</UI.Text>
</UI.Row>
