<script lang="ts">
  import type { Workspace } from "../../app/access"
  import PageBody from "../../components/PageBody.svelte"
  import PageHeading from "../../components/PageHeading.svelte"
  import RouterLink from "../../components/RouterLink.svelte"

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
    <p class="eyebrow">Workspace</p>
    <h1>{workspace.name ?? workspace.id}</h1>
    <p class="subtitle">{workspace.description ?? "\u00a0"}</p>
  </PageHeading>
  <section class="grid">
    <section class="card surface stack">
      <div class="split">
        <h2>Latest Chats</h2>
        <button
          class="primary small"
          type="button"
          onclick={() => void onCreateSession()}>New chat</button
        >
      </div>
      <div class="list">
        {#each sessions as session}
          <RouterLink
            class="list-entry"
            href={`${sessionsPath()}/${encodeURIComponent(session.id)}`}
            ><span class="stack" style:--space="calc(var(--space) / 4)"
              ><span class="truncate">{session.name ?? "New Chat"}</span><small
                class="muted truncate"
                ><time datetime={session.created_at}
                  >{createdAtLabel(session.created_at)}</time
                >{#if session.project !== undefined} / {session.project.name ?? "New Project"}{/if}</small
              ></span
            ></RouterLink
          >
        {:else}<p class="notice">No chats yet.</p>{/each}
      </div>
      {#if sessions.length > 0}<RouterLink
          class="navigation-link"
          href={sessionsPath()}>View all chats</RouterLink
        >{/if}
    </section>
    <section class="card surface stack">
      <div class="split">
        <h2>Latest Projects</h2>
        <button
          class="primary small"
          type="button"
          disabled={creatingProject}
          onclick={() => void onCreateProject()}>New project</button
        >
      </div>
      <div class="list">
        {#each projects as project}
          <RouterLink
            class="list-entry"
            href={`${projectsPath()}/${encodeURIComponent(project.id)}`}
            ><span class="stack" style:--space="calc(var(--space) / 4)"
              ><span class="truncate">{project.name ?? "New Project"}</span><small
                class="muted truncate"
                ><time datetime={project.created_at}
                  >{createdAtLabel(project.created_at)}</time
                ></small
              ></span
            ></RouterLink
          >
        {:else}<p class="notice">No projects yet.</p>{/each}
      </div>
      {#if projects.length > 0}<RouterLink
          class="navigation-link"
          href={projectsPath()}>View all projects</RouterLink
        >{/if}
    </section>
    {#if error !== ""}<p class="notice action" role="alert" aria-live="polite">
        {error}
      </p>{/if}
  </section>
</PageBody>
