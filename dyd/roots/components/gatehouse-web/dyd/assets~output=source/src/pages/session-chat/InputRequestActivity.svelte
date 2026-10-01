<script lang="ts">
  import { CircleCheck, CircleX, ClipboardList } from "@lucide/svelte"
  import type { ChatEventTree } from "../../app/chat"
  import { inputResponse } from "./chat-controller.svelte"

  let { request }: { request: ChatEventTree<"input.request"> } = $props()
  const response = $derived(inputResponse(request))
</script>

<section class="event-request" data-resolved={response !== undefined || undefined}>
  {#if response?.event.kind === "input.success"}
    <CircleCheck size={15} strokeWidth={2} aria-hidden="true" />
    <strong>Input submitted:</strong>
  {:else if response?.event.kind === "input.failure"}
    <CircleX size={15} strokeWidth={2} aria-hidden="true" />
    <strong>{response.event.payload.code === "cancelled" ? "Input cancelled:" : "Input failed:"}</strong>
  {:else}
    <ClipboardList size={15} strokeWidth={2} aria-hidden="true" />
    <strong>Input requested:</strong>
  {/if}
  <span>{request.event.payload.description ?? "Provide details"}</span>
</section>
