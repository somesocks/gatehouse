<script lang="ts">
  import { RouterLink } from "../../../ui"
  import { useRuntime } from "../../../app/runtime.svelte"
  const runtime = useRuntime()
  import { systemNavigation } from "../../../app/navigation"
  import * as UI from "../../../ui"
</script>

<UI.AppFrame
  navigation={systemNavigation(runtime, "overview")}
  breadcrumbs={[
    { label: "System", href: "/app/system" },
    { label: "Overview" },
  ]}
>
  <UI.Stack as="section">
    <UI.Stack as="hgroup">
      <UI.Text as="p" size="eyebrow">System</UI.Text>
      <UI.Heading level={2}>System administration</UI.Heading>
      <UI.Text as="p">Manage global Gatehouse state.</UI.Text>
    </UI.Stack>
    <RouterLink variant="item" href="/app/system/principals"
      ><UI.Stack as="span"
        ><UI.Text as="strong" emphasis="strong">Principals</UI.Text><UI.Text
          as="small"
          size="small"
          >View and enable or disable principals and their identities.</UI.Text
        ></UI.Stack
      ></RouterLink
    >
    <RouterLink variant="item" href="/app/system/grants"
      ><UI.Stack as="span"
        ><UI.Text as="strong" emphasis="strong">System grants</UI.Text><UI.Text
          as="small"
          size="small">Grant or revoke system-manager access.</UI.Text
        ></UI.Stack
      ></RouterLink
    >
    <RouterLink variant="item" href="/app/system/agent-providers"
      ><UI.Stack as="span"
        ><UI.Text as="strong" emphasis="strong">Agent providers</UI.Text
        ><UI.Text as="small" size="small"
          >Configure model-provider protocols and credentials.</UI.Text
        ></UI.Stack
      ></RouterLink
    >
    <RouterLink variant="item" href="/app/system/agent-models"
      ><UI.Stack as="span"
        ><UI.Text as="strong" emphasis="strong">Agent models</UI.Text><UI.Text
          as="small"
          size="small"
          >Configure models available for workspace bindings.</UI.Text
        ></UI.Stack
      ></RouterLink
    >
    <RouterLink variant="item" href="/app/system/storage-providers"
      ><UI.Stack as="span"
        ><UI.Text as="strong" emphasis="strong">Storage providers</UI.Text
        ><UI.Text as="small" size="small"
          >Configure embedded or S3 storage.</UI.Text
        ></UI.Stack
      ></RouterLink
    >
    <RouterLink variant="item" href="/app/system/workspaces"
      ><UI.Stack as="span"
        ><UI.Text as="strong" emphasis="strong">Workspaces</UI.Text><UI.Text
          as="small"
          size="small">View all workspaces and create new ones.</UI.Text
        ></UI.Stack
      ></RouterLink
    >
    <RouterLink variant="item" href="/app/system/workspace-agent-bindings"
      ><UI.Stack as="span"
        ><UI.Text as="strong" emphasis="strong"
          >Workspace agent bindings</UI.Text
        ><UI.Text as="small" size="small"
          >Assign agent models to workspaces.</UI.Text
        ></UI.Stack
      ></RouterLink
    >
    <RouterLink variant="item" href="/app/system/workspace-storage-bindings"
      ><UI.Stack as="span"
        ><UI.Text as="strong" emphasis="strong"
          >Workspace storage bindings</UI.Text
        ><UI.Text as="small" size="small"
          >Assign storage providers to workspaces.</UI.Text
        ></UI.Stack
      ></RouterLink
    >
  </UI.Stack>
</UI.AppFrame>
