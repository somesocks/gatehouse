<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

  import { createWorkspaceAgentBindingController } from "./workspace-agent-binding-controller.svelte"

  let { workspaceID, bindingID }: { workspaceID: string; bindingID: string } =
    $props()
  const runtime = useRuntime()
  const controller = untrack(() =>
    createWorkspaceAgentBindingController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )
  $effect(() => {
    void controller.load(workspaceID, bindingID)
  })
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "workspace-agent-bindings")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Workspace agent binding" },
  ]}
>
  <UI.Stack as="section">
    {#if controller.state.loading}
      <UI.Text as="p" tone="muted">Loading binding...</UI.Text>
    {:else if controller.state.binding === null}
      <UI.Text as="p" size="small" role="alert">
        {controller.state.error || "Workspace agent binding was not found."}
      </UI.Text>
      <RouterLink variant="outline" href="/app/system/workspace-agent-bindings"
        >Back to bindings</RouterLink
      >
    {:else if !controller.state.editing}
      <UI.Row justify="between">
        <UI.Group>
          <UI.Text as="p" size="eyebrow">Workspace agent binding</UI.Text>
          <UI.Heading level={2}>
            {controller.state.binding.workspace} / {controller.state.binding
              .alias}
          </UI.Heading>
        </UI.Group>
        <UI.Button
          variant="outline"
          type="button"
          onclick={() => controller.edit()}>Edit</UI.Button
        >
      </UI.Row>
      <UI.Stack as="dl">
        <UI.Field>
          <UI.Text as="dt" size="small">Automatic replies</UI.Text>
          <UI.Text as="dd"
            >{controller.state.binding.default
              ? "Default agent"
              : "Not default"}</UI.Text
          >
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Label</UI.Text>
          <UI.Text as="dd"
            >{controller.state.binding.label ?? "Not configured"}</UI.Text
          >
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Custom system prompt</UI.Text>
          <UI.Text as="dd"
            >{controller.state.binding.system_prompt ??
              "Not configured"}</UI.Text
          >
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Lisp prelude</UI.Text>
          <UI.Text as="dd"
            >{controller.state.binding.prelude ?? "Standard prelude"}</UI.Text
          >
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Token rate limits</UI.Text>
          <UI.Text as="dd"
            >{controller.state.binding.rate_limits === undefined
              ? "Not configured"
              : JSON.stringify(controller.state.binding.rate_limits)}</UI.Text
          >
        </UI.Field>
        <UI.Field>
          <UI.Text as="dt" size="small">Status</UI.Text>
          <UI.Text as="dd"
            >{controller.state.binding.enabled
              ? "Enabled"
              : "Disabled"}</UI.Text
          >
        </UI.Field>
      </UI.Stack>
    {:else}
      <UI.Stack>
        <UI.Group>
          <UI.Text as="p" size="eyebrow">Workspace agent binding</UI.Text>
          <UI.Heading level={2}>Edit binding</UI.Heading>
        </UI.Group>
      </UI.Stack>
      <UI.Form
        onsubmit={(event) => {
          event.preventDefault()
          void controller.update()
        }}
      >
        <UI.Field>
          <UI.Label for="agent-binding-workspace">Workspace ID</UI.Label>
          <UI.Group>
            <UI.Input
              id="agent-binding-workspace"
              disabled
              value={controller.state.form.workspace}
            />
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="agent-binding-alias">Alias</UI.Label>
          <UI.Group>
            <UI.Input
              id="agent-binding-alias"
              disabled
              value={controller.state.form.alias}
            />
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="agent-binding-model">Model ID</UI.Label>
          <UI.Group>
            <UI.Input
              id="agent-binding-model"
              bind:value={controller.state.form.model}
            />
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="agent-binding-label">Label</UI.Label>
          <UI.Group>
            <UI.Input
              id="agent-binding-label"
              bind:value={controller.state.form.label}
            />
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="agent-binding-system-prompt"
            >Custom system prompt</UI.Label
          >
          <UI.Group>
            <UI.Textarea
              id="agent-binding-system-prompt"
              rows={4}
              bind:value={controller.state.form.systemPrompt}
            ></UI.Textarea>
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="agent-binding-prelude">Lisp prelude</UI.Label>
          <UI.Group>
            <UI.Textarea
              id="agent-binding-prelude"
              rows={4}
              bind:value={controller.state.form.prelude}
            ></UI.Textarea>
          </UI.Group>
        </UI.Field>
        <UI.Field>
          <UI.Label for="agent-binding-rate-limits">Token rate limits</UI.Label>
          <UI.Group>
            <UI.Textarea
              id="agent-binding-rate-limits"
              rows={10}
              placeholder={'{"workspace_input":{"minimum_balance":-60000,"maximum_balance":120000,"refill_per_minute":60000}}'}
              bind:value={controller.state.form.rateLimits}
            ></UI.Textarea>
          </UI.Group>
          <UI.Text as="p" size="small"
            >Optional JSON with workspace/user input/output buckets.</UI.Text
          >
        </UI.Field>
        <UI.Field>
          <UI.Label layout="choice"
            ><UI.Checkbox bind:checked={controller.state.form.default} /> Default
            automatic-reply agent</UI.Label
          >
        </UI.Field>
        <UI.Field>
          <UI.Label layout="choice"
            ><UI.Checkbox bind:checked={controller.state.form.enabled} /> Enabled</UI.Label
          >
        </UI.Field>
        <UI.Row>
          <UI.Group>
            <UI.Button variant="default" type="submit">Save changes</UI.Button>
          </UI.Group>
          <UI.Group>
            <UI.Button
              variant="outline"
              type="button"
              onclick={() => (controller.state.editing = false)}
              >Cancel</UI.Button
            >
          </UI.Group>
        </UI.Row>
      </UI.Form>
      {#if controller.state.error}<UI.Text as="p" size="small" role="alert">
          {controller.state.error}
        </UI.Text>{/if}
    {/if}
  </UI.Stack>
</UI.AppFrame>
