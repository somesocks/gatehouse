<script lang="ts">
  import * as UI from "../../../ui"
  import type { createCodexLoginController } from "./codex-login-controller.svelte"

  let {
    state,
    copyError = "",
    onstart,
    oncopy,
    oncancel,
    onclose,
  }: {
    state: ReturnType<typeof createCodexLoginController>["state"]
    copyError?: string
    onstart: () => void
    oncopy: () => void
    oncancel: () => void
    onclose: () => void
  } = $props()

  const descriptions = {
    idle: "Sign in to authorize access to Codex.",
    starting: "Preparing your sign-in…",
    pending: "Enter this code on OpenAI’s website to continue.",
    succeeded: undefined,
    failed: undefined,
    expired: undefined,
    cancelled: undefined,
  }
  const announcements = {
    idle: "",
    starting: "Preparing your sign-in.",
    pending: "Waiting for approval.",
    succeeded: "Login verified.",
    failed: "",
    expired: "This code has expired.",
    cancelled: "Sign-in cancelled.",
  }
</script>

<UI.StatusPage
  width="sm"
  centered
  centerContent
  title="ChatGPT account"
  description={descriptions[state.phase]}
>
  {#snippet children()}
    <UI.Text hidden role="status" aria-live="polite" aria-atomic="true"
      >{announcements[state.phase]}</UI.Text
    >
    <UI.Stack>
      {#if state.phase === "idle"}
        <UI.Button onclick={onstart}>Continue with ChatGPT</UI.Button>
      {:else if state.phase === "starting"}
        <UI.Button disabled aria-busy="true"
          ><UI.Spinner aria-hidden="true" />Starting…</UI.Button
        >
        <UI.Button variant="link" onclick={oncancel}>Cancel</UI.Button>
      {:else if state.phase === "pending" && state.attempt}
        <UI.CopyableCode value={state.attempt.user_code} {oncopy} />
        {#if copyError}<UI.Text
            as="p"
            size="small"
            tone="destructive"
            role="alert">{copyError}</UI.Text
          >{/if}
        <UI.Link
          variant="default"
          href={state.attempt.verification_uri}
          target="_blank"
          rel="noopener noreferrer">Open OpenAI</UI.Link
        >
        <UI.Stack gap="small">
          <UI.Text as="p" tone="muted" size="small"
            >Waiting for approval…</UI.Text
          >
          {#if state.remainingMinutes !== null}<UI.Text
              as="p"
              tone="muted"
              size="small"
            >
              {state.remainingMinutes > 1
                ? `Expires in ${state.remainingMinutes} minutes`
                : "Expires in less than a minute"}
            </UI.Text>{/if}
        </UI.Stack>
        <UI.Button variant="link" onclick={oncancel}>Cancel</UI.Button>
      {:else if state.phase === "succeeded"}
        <UI.Row justify="center"
          ><UI.CircleCheck size={32} aria-hidden="true" /></UI.Row
        >
        <UI.Heading level={3}>Login verified</UI.Heading>
        {#if state.result?.account.email}<UI.Text as="p"
            >{state.result.account.email}</UI.Text
          >{/if}
        <UI.Text as="p" tone="muted">You can close this tab.</UI.Text>
        <UI.Button onclick={onclose}>Close tab</UI.Button>
      {:else if state.phase === "failed"}
        <UI.Row justify="center"
          ><UI.Text tone="destructive"
            ><UI.CircleX size={32} aria-hidden="true" /></UI.Text
          ></UI.Row
        >
        <UI.Heading level={3}>Couldn’t complete sign-in.</UI.Heading>
        <UI.Text as="p" role="alert"
          >{state.error || "Please try again."}</UI.Text
        >
        <UI.Button onclick={onstart}>Try again</UI.Button>
        <UI.Button variant="link" onclick={onclose}>Close tab</UI.Button>
      {:else if state.phase === "expired"}
        <UI.Heading level={3}>This code has expired.</UI.Heading>
        <UI.Text as="p" tone="muted">Start again to get a new code.</UI.Text>
        <UI.Button onclick={onstart}>Start again</UI.Button>
        <UI.Button variant="link" onclick={onclose}>Close tab</UI.Button>
      {:else if state.phase === "cancelled"}
        <UI.Heading level={3}>Sign-in cancelled.</UI.Heading>
        <UI.Button onclick={onstart}>Start again</UI.Button>
        <UI.Button variant="link" onclick={onclose}>Close tab</UI.Button>
      {/if}
    </UI.Stack>
  {/snippet}
</UI.StatusPage>
