<script lang="ts">
  import { onMount, untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import { createCodexLoginController } from "./codex-login-controller.svelte"
  import CodexLoginCard from "./CodexLoginCard.svelte"
  import * as UI from "../../../ui"

  let { providerID }: { providerID: string } = $props()

  const runtime = useRuntime()
  const controller = untrack(() =>
    createCodexLoginController({
      providerID,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  let copyError = $state("")
  onMount(() => () => controller.stop())

  function start() {
    copyError = ""
    void controller.start()
  }

  async function copyCode() {
    const attempt = controller.state.attempt
    if (!attempt) return
    copyError = ""
    try {
      await navigator.clipboard.writeText(attempt.user_code)
    } catch {
      if (controller.state.attempt === attempt) {
        copyError = "Couldn’t copy the code. Select it and copy it manually."
      }
    }
  }
</script>

{#if providerID}<CodexLoginCard
    state={controller.state}
    {copyError}
    onstart={start}
    oncopy={() => void copyCode()}
    oncancel={() => controller.cancel()}
    onclose={() => window.close()}
  />{:else}
  <UI.StatusPage
    width="sm"
    centered
    centerContent
    title="ChatGPT account"
    description="Choose an OpenAI Codex provider to connect."
  >
    <UI.Link variant="default" href="/app/system/agent-providers"
      >Choose provider</UI.Link
    >
  </UI.StatusPage>
{/if}
