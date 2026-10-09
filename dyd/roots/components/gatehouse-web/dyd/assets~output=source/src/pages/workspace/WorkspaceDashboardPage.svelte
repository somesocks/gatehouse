<script lang="ts">
  import { PageBody, PageHeading, RouterLink } from "../../ui"
  import * as UI from "../../ui"
  import type { Workspace } from "../../app/access"

  type Session = {
    id: string
    created_at: string
    name?: string
    project?: { id: string; name?: string }
  }
  type Project = { id: string; created_at: string; name?: string }

  let {
    workspace,
    sessions,
    projects,
    creatingProject,
    error,
    onCreateSession,
    onCreateProject,
  }: {
    workspace: Workspace
    sessions: Session[]
    projects: Project[]
    creatingProject: boolean
    error: string
    onCreateSession: () => void | Promise<void>
    onCreateProject: () => void | Promise<void>
  } = $props()

  function sessionsPath() {
    return `/app/wsp/${encodeURIComponent(workspace.id)}/ses`
  }

  function projectsPath() {
    return `/app/wsp/${encodeURIComponent(workspace.id)}/prj`
  }

  function createdAtLabel(value: string) {
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) {
      return value
    }
    const number = (part: number) => part.toString().padStart(2, "0")
    return `${date.getFullYear()}-${number(date.getMonth() + 1)}-${number(date.getDate())} ${number(date.getHours())}:${number(date.getMinutes())}`
  }
</script>

<PageBody fluid>
  <PageHeading as="header">
    <UI.Text as="p" size="eyebrow">Workspace</UI.Text>
    <UI.Heading level={1}>{workspace.name ?? workspace.id}</UI.Heading>
    <UI.Text as="p" tone="muted">{workspace.description ?? "\u00a0"}</UI.Text>
  </PageHeading>
  <UI.Grid>
    <UI.Panel>
      <UI.Row justify="between">
        <UI.Heading level={2}>Latest Chats</UI.Heading>
        <UI.Button
          variant="default"
          size="sm"
          type="button"
          onclick={() => void onCreateSession()}>New chat</UI.Button
        >
      </UI.Row>
      <UI.List>
        {#each sessions as session}
          <RouterLink
            variant="entry"
            href={`${sessionsPath()}/${encodeURIComponent(session.id)}`}
            ><UI.Stack as="span" gap="tight"
              ><UI.Text truncate>{session.name ?? "New Chat"}</UI.Text><UI.Text
                as="small"
                size="small"
                tone="muted"
                truncate
                ><UI.Text as="time" datetime={session.created_at}
                  >{createdAtLabel(session.created_at)}</UI.Text
                >{#if session.project !== undefined}
                  / {session.project.name ?? "New Project"}{/if}</UI.Text
              ></UI.Stack
            ></RouterLink
          >
        {:else}<UI.Notice>No chats yet.</UI.Notice>{/each}
      </UI.List>
      {#if sessions.length > 0}<RouterLink href={sessionsPath()}
          >View all chats</RouterLink
        >{/if}
    </UI.Panel>
    <UI.Panel>
      <UI.Row justify="between">
        <UI.Heading level={2}>Latest Projects</UI.Heading>
        <UI.Button
          variant="default"
          size="sm"
          type="button"
          disabled={creatingProject}
          onclick={() => void onCreateProject()}>New project</UI.Button
        >
      </UI.Row>
      <UI.List>
        {#each projects as project}
          <RouterLink
            variant="entry"
            href={`${projectsPath()}/${encodeURIComponent(project.id)}`}
            ><UI.Stack as="span" gap="tight"
              ><UI.Text truncate>{project.name ?? "New Project"}</UI.Text
              ><UI.Text as="small" size="small" tone="muted" truncate
                ><UI.Text as="time" datetime={project.created_at}
                  >{createdAtLabel(project.created_at)}</UI.Text
                ></UI.Text
              ></UI.Stack
            ></RouterLink
          >
        {:else}<UI.Notice>No projects yet.</UI.Notice>{/each}
      </UI.List>
      {#if projects.length > 0}<RouterLink href={projectsPath()}
          >View all projects</RouterLink
        >{/if}
    </UI.Panel>
    {#if error !== ""}<UI.Notice
        variant="destructive"
        role="alert"
        aria-live="polite"
      >
        {error}
      </UI.Notice>{/if}
  </UI.Grid>
</PageBody>
