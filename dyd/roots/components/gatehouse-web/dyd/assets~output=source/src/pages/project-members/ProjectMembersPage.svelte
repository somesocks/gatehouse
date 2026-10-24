<script lang="ts">
  import { onMount } from "svelte"
  import * as UI from "../../ui"
  import { useRuntime } from "../../app/runtime.svelte"
  import {
    projectMemberName,
    projectRoles,
    type ProjectMemberKind,
  } from "../../app/project-members"
  import { createProjectAccessController } from "./project-access-controller.svelte"
  import ProjectMemberDialogs from "./ProjectMemberDialogs.svelte"
  import { roleLabel } from "../../app/roles"

  let {
    workspaceID,
    projectID,
    onAccessLost,
  }: {
    workspaceID: string
    projectID: string
    onAccessLost: () => void
  } = $props()
  const runtime = useRuntime()
  const controller = createProjectAccessController({
    activity: runtime.activity,
    onAuthenticationLost: runtime.requireLogin,
    onAccessLost: () => onAccessLost(),
  })
  const sections: { kind: ProjectMemberKind; title: string; noun: string }[] = [
    { kind: "principal", title: "People", noun: "person" },
    { kind: "group", title: "Groups", noun: "group" },
  ]

  onMount(() => {
    controller.start(workspaceID, projectID)
    void controller.open()
    return controller.stop
  })
</script>

<UI.ManagementLayout>
  {#snippet header()}
    <UI.AdminHeader
      kind="Project"
      title="Members"
    />
    {#if !controller.state.canManage && controller.people.state.hasLoaded}
      <UI.Text tone="muted">Only project admins can change membership.</UI.Text>
    {/if}
    {#if controller.state.error && !controller.state.adding && !controller.state.action}
      <UI.Notice variant="destructive" role="alert">
        {controller.state.error}
        <UI.Button variant="outline" onclick={() => void controller.reload()}>
          Reload members
        </UI.Button>
      </UI.Notice>
    {/if}
  {/snippet}

  {#each sections as section (section.kind)}
    {@const directory = controller.directory(section.kind)}
    <UI.ManagementSection title={section.title} loading={directory.state.busy}>
      {#snippet actions()}
        {#if controller.state.canManage}
          <UI.Button
            size="sm"
            variant="outline"
            disabled={controller.state.saving}
            onclick={() => controller.beginAdd(section.kind)}
          >Add {section.noun}</UI.Button>
        {/if}
      {/snippet}
      {#snippet toolbar()}
        <UI.SearchToolbar
          framed={false}
          label={`Search ${section.title.toLowerCase()}`}
          placeholder={`Search ${section.title.toLowerCase()}…`}
          bind:value={directory.state.search}
          loading={directory.state.busy}
          onsearch={() => void directory.search()}
        />
      {/snippet}
      <UI.RecordList
        label={`Project ${section.title.toLowerCase()}`}
        framed={false}
        scrollable={false}
        loading={directory.state.busy}
        items={directory.state.items.map((item) => ({
          ...item,
          label: projectMemberName(item),
          disabled: !item.enabled,
        }))}
        emptyMessage={directory.state.loading
          ? `Loading ${section.title.toLowerCase()}…`
          : directory.state.hasLoaded
            ? `No matching ${section.title.toLowerCase()}.`
            : "Members could not be loaded."}
      >
        {#snippet details(item)}{#if item.alias}@{item.alias}{/if}{/snippet}
        {#snippet actions(item)}
          <UI.Stack>
            {#each item.grants as grant (grant.id)}
              <UI.Row justify="end" wrap={false}>
                <UI.Text size="small" tone="muted">
                  {projectRoles.find((role) => role.value === grant.role)?.label}
                </UI.Text>
                {#if controller.state.canManage}
                  <UI.Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={`Edit ${roleLabel(grant.role)} role for ${item.label}`}
                    title="Edit role"
                    disabled={controller.state.saving}
                    onclick={() => controller.beginAction("role", section.kind, grant, item.label)}
                  >
                    <UI.Pencil aria-hidden="true" />
                  </UI.Button>
                  <UI.Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={`Remove ${roleLabel(grant.role)} grant for ${item.label}`}
                    title="Remove grant"
                    disabled={controller.state.saving}
                    onclick={() => controller.beginAction("remove", section.kind, grant, item.label)}
                  >
                    <UI.Trash2 aria-hidden="true" />
                  </UI.Button>
                {/if}
              </UI.Row>
            {/each}
          </UI.Stack>
        {/snippet}
      </UI.RecordList>
      {#snippet footer()}
        <UI.ActionFeedback
          error={directory.state.error}
          pending={directory.state.busy}
          label={`Refreshing ${section.title.toLowerCase()}…`}
        />
        {#if directory.state.error}
          <UI.Row>
            <UI.Button
              variant="outline"
              size="sm"
              onclick={() => void directory.retry()}
            >Reload {section.title.toLowerCase()}</UI.Button>
          </UI.Row>
        {/if}
        <UI.PaginationFooter
          showCount={false}
          previous={directory.state.history.length > 0}
          next={!!directory.state.next}
          loading={directory.state.busy}
          onprevious={() => void directory.previous()}
          onnext={() => void directory.next()}
        />
      {/snippet}
    </UI.ManagementSection>
  {/each}
</UI.ManagementLayout>

<ProjectMemberDialogs {controller} />
