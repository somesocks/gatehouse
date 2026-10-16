<script lang="ts">
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
  <UI.AdminContent mode="hub">
    <UI.AdminHeader
      title="System administration"
      description="Manage access, shared infrastructure, and workspaces."
    />
    <UI.Stack>
      <UI.Heading level={3}>Access</UI.Heading>
      <UI.NavigationGrid>
        <UI.NavigationCard
          title="Principals"
          description="Create principals and manage login identities and account status."
          href="/app/system/principals"
        />
        <UI.NavigationCard
          title="System grants"
          description="Grant or revoke system-manager access."
          href="/app/system/grants"
        />
      </UI.NavigationGrid>
    </UI.Stack>
    <UI.Stack>
      <UI.Heading level={3}>Infrastructure</UI.Heading>
      <UI.NavigationGrid>
        <UI.NavigationCard
          title="Agent providers"
          description="Configure model-provider connections and credentials."
          href="/app/system/agent-providers"
        />
        <UI.NavigationCard
          title="Agent models"
          description="Configure model selection and generation settings."
          href="/app/system/agent-models"
        />
        <UI.NavigationCard
          title="Storage providers"
          description="Configure embedded or S3 storage."
          href="/app/system/storage-providers"
        />
      </UI.NavigationGrid>
    </UI.Stack>
    <UI.Stack>
      <UI.Heading level={3}>Workspaces</UI.Heading>
      <UI.NavigationGrid>
        <UI.NavigationCard
          title="Workspaces"
          description="Manage workspace details, principal access, and bindings."
          href="/app/system/workspaces"
        />
      </UI.NavigationGrid>
    </UI.Stack>
  </UI.AdminContent>
</UI.AppFrame>
