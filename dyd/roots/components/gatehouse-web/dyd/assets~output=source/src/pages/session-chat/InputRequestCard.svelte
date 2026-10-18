<script lang="ts">
  import * as UI from "../../ui"
  import { untrack } from "svelte"
  import { fetchChatInputLaunch, type ChatEventTree } from "../../app/chat"
  import InputForm from "../input-form/InputForm.svelte"
  import { createInputTransport } from "../input-form/input-api"
  import { createInputFormController } from "../input-form/input-controller.svelte"
  import { inputResponse } from "./chat-controller.svelte"

  let {
    request,
    workspaceID,
    sessionID,
    onAuthenticationLost,
    onResolved,
  }: {
    request: ChatEventTree<"input.request">
    workspaceID: string
    sessionID: string
    onAuthenticationLost: () => void
    onResolved: () => void
  } = $props()
  const inputID = $derived(request.event.ref.id)
  const response = $derived(inputResponse(request))
  const responseKind = $derived(response?.event.kind)
  type FormController = ReturnType<typeof createInputFormController>
  let controller = $state<FormController | null>(null)
  let formStatus = $state<
    "loading" | "ready" | "error" | "denied" | "resolved"
  >("loading")
  let retry = $state(0)
  const localStatus = $derived(controller?.state.status)
  const resolved = $derived(
    response !== undefined ||
      formStatus === "resolved" ||
      localStatus === "submitted" ||
      localStatus === "cancelled" ||
      localStatus === "resolved",
  )

  async function load(
    signal: AbortSignal,
    workspace: string,
    session: string,
    id: string,
    opened: (value: FormController) => void,
  ): Promise<void> {
    formStatus = "loading"
    controller = null
    try {
      const launched = await fetchChatInputLaunch(
        workspace,
        session,
        id,
        signal,
      )
      if (signal.aborted) return
      if (launched.status === 401) {
        onAuthenticationLost()
        return
      }
      if (launched.status === 409) {
        formStatus = "resolved"
        onResolved()
        return
      }
      if (launched.status === 403 || launched.status === 404) {
        formStatus = "denied"
        return
      }
      if (!launched.ok) throw new Error("input could not be opened")
      const result = (await launched.json()) as { capability: string }
      if (signal.aborted) return
      const capability = result.capability
      if (typeof capability !== "string" || capability === "")
        throw new Error("invalid input capability")
      const form = createInputFormController(createInputTransport(capability))
      opened(form)
      controller = form
      formStatus = "ready"
      await form.load()
    } catch {
      if (!signal.aborted) formStatus = "error"
    }
  }

  $effect(() => {
    const id = inputID
    const kind = responseKind
    const workspace = workspaceID
    const session = sessionID
    const attempt = retry
    if (kind !== undefined) {
      controller = null
      return
    }
    const abort = new AbortController()
    let form: FormController | null = null
    void load(abort.signal, workspace, session, id, (value) => {
      form = value
    })
    return () => {
      abort.abort()
      form?.dispose()
    }
  })

  $effect(() => {
    if (
      localStatus === "submitted" ||
      localStatus === "cancelled" ||
      localStatus === "resolved"
    )
      untrack(onResolved)
  })
</script>

{#if !resolved}
  <UI.Message
    purpose="form"
    aria-label={request.event.payload.description ?? "Input form"}
    aria-busy={formStatus === "loading" || localStatus === "loading"}
  >
    <UI.Heading level={2}
      >{controller?.state.form?.title ??
        request.event.payload.description ??
        "Input form"}</UI.Heading
    >
    {#if formStatus === "ready" && controller !== null}
      <InputForm {controller} {inputID} depth={2} />
    {:else if formStatus === "denied"}
      <UI.Text as="p" role="alert">Unavailable or already resolved.</UI.Text>
    {:else if formStatus === "error"}
      <UI.Text as="p" role="alert">The form could not be loaded.</UI.Text>
      <UI.Button variant="outline" type="button" onclick={() => (retry += 1)}
        >Try again</UI.Button
      >
    {/if}
  </UI.Message>
{/if}
