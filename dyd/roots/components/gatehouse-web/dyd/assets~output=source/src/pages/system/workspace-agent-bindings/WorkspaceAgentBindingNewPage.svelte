<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"

  import { createWorkspaceAgentBindingsController } from "./workspace-agent-bindings-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() =>
    createWorkspaceAgentBindingsController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
    }),
  )

  async function create(): Promise<void> {
    await controller.create()
    if (controller.state.error === "")
      runtime.navigate("/app/system/workspace-agent-bindings", true)
  }
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "workspace-agent-bindings")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "New workspace agent binding" },
  ]}
>
  <UI.Stack as="section">
    <UI.Stack>
      <UI.Group>
        <UI.Text as="p" size="eyebrow">System</UI.Text>
        <UI.Heading level={2}>New workspace agent binding</UI.Heading>
      </UI.Group>
    </UI.Stack>
    <UI.Form
      onsubmit={(event) => {
        event.preventDefault()
        void create()
      }}
    >
      <UI.Field>
        <UI.Label for="agent-binding-workspace">Workspace ID</UI.Label>
        <UI.Group>
          <UI.Input
            id="agent-binding-workspace"
            required
            bind:value={controller.state.form.workspace}
          />
        </UI.Group>
      </UI.Field>
      <UI.Field>
        <UI.Label for="agent-binding-alias">Binding alias</UI.Label>
        <UI.Group>
          <UI.Input
            id="agent-binding-alias"
            required
            bind:value={controller.state.form.alias}
          />
        </UI.Group>
      </UI.Field>
      <UI.Field>
        <UI.Label for="agent-binding-model">Model ID</UI.Label>
        <UI.Group>
          <UI.Input
            id="agent-binding-model"
            required
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
          ><UI.Checkbox bind:checked={controller.state.form.default} /> Default automatic-reply
          agent</UI.Label
        >
      </UI.Field>
      <UI.Row>
        <UI.Group>
          <UI.Button variant="default" type="submit">Add binding</UI.Button>
        </UI.Group>
        <UI.Group>
          <RouterLink
            variant="outline"
            href="/app/system/workspace-agent-bindings">Cancel</RouterLink
          >
        </UI.Group>
      </UI.Row>
    </UI.Form>
    {#if controller.state.error}<UI.Text as="p" size="small" role="alert">
        {controller.state.error}
      </UI.Text>{/if}
  </UI.Stack>
</UI.AppFrame>
