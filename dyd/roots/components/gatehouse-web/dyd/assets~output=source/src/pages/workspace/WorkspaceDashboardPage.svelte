<script lang="ts">
  import * as UI from "../../ui"
  import type { Workspace } from "../../app/access"

  let { workspace }: { workspace: Workspace } = $props()
  const base = $derived(`/app/wsp/${encodeURIComponent(workspace.id)}`)
</script>

<UI.AdminContent mode="hub">
  <UI.AdminHeader
    kind="Workspace"
    title={workspace.name ?? "Workspace"}
    description={workspace.description}
  />
  <UI.NavigationGrid>
    <UI.NavigationCard
      title="Chats"
      description="Browse conversations and work with workspace agents."
      href={`${base}/ses`}
    />
    <UI.NavigationCard
      title="Projects"
      description="Browse projects and their shared resources."
      href={`${base}/prj`}
    />
    <UI.NavigationCard
      title="Groups"
      description="Inspect your workspace group memberships."
      href={`${base}/grp`}
    />
  </UI.NavigationGrid>
</UI.AdminContent>
