<script lang="ts">
  import { CircleCheck, CircleX, ClipboardList } from "@lucide/svelte"
  import { fetchChatInputLaunch, type ChatEventTree } from "../../app/chat"
  import { inputResponse } from "./chat-controller.svelte"

  let {
    request,
    workspaceID,
    sessionID,
    onAuthenticationLost,
  }: {
    request: ChatEventTree
    workspaceID: string
    sessionID: string
    onAuthenticationLost: () => void
  } = $props()
  const inputID = $derived(request.event.ref.id)
  const response = $derived(inputResponse(request))
  const responseKind = $derived(response?.event.kind)
  let url = $state<string | null>(null)
  let state = $state<"loading" | "ready" | "error" | "denied">("loading")

  async function load(signal: AbortSignal, id: string): Promise<void> {
    state = "loading"
    url = null
    try {
      const launched = await fetchChatInputLaunch(
        workspaceID,
        sessionID,
        id,
        signal,
      )
      if (signal.aborted) return
      if (launched.status === 401) {
        onAuthenticationLost()
        return
      }
      if (launched.status === 403) {
        state = "denied"
        return
      }
      if (launched.status === 409) {
        state = "denied"
        return
      }
      if (!launched.ok) throw new Error("input could not be opened")
      const result = (await launched.json()) as { url: string }
      if (signal.aborted) return
      const parsed = new URL(result.url)
      if (!parsed.hash.startsWith("#capability=")) throw new Error("invalid input link")
      url = parsed.href
      state = "ready"
    } catch {
      if (!signal.aborted) state = "error"
    }
  }

  $effect(() => {
    const id = inputID
    const kind = responseKind
    if (kind !== undefined) {
      url = null
      return
    }
    const abort = new AbortController()
    void load(abort.signal, id)
    return () => abort.abort()
  })
</script>

<section class="event-request" data-resolved={response !== undefined || undefined}>
  {#if responseKind === "input.success"}
    <CircleCheck size={15} strokeWidth={2} aria-hidden="true" />
    <strong>Input submitted:</strong>
  {:else if responseKind === "input.failure"}
    <CircleX size={15} strokeWidth={2} aria-hidden="true" />
    <strong>{response?.event.payload.code === "cancelled" ? "Input cancelled:" : "Input failed:"}</strong>
  {:else}
    <ClipboardList size={15} strokeWidth={2} aria-hidden="true" />
    <strong>Input required:</strong>
  {/if}
  <span>{request.event.payload.description ?? "Provide details"}</span>
  {#if response === undefined && state === "ready" && url !== null}
    <span data-actions>
      <a class="primary input-open" href={url} target="_blank" rel="noopener noreferrer" referrerpolicy="no-referrer">Open form</a>
    </span>
  {:else if response === undefined && state === "loading"}
    <small>Preparing link...</small>
  {:else if response === undefined && state === "denied"}
    <small>Unavailable or already resolved.</small>
  {:else if response === undefined}
    <small>Could not open form. Refresh chat to try again.</small>
  {/if}
</section>
