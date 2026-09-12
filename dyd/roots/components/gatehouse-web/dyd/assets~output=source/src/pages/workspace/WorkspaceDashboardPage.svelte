<script lang="ts">
  import type { Workspace } from "../../app/access"
  import RouterLink from "../../components/RouterLink.svelte"

  type Session = { id: string; created_at: string; name?: string; project?: { id: string; name?: string } }
  type Project = { id: string; created_at: string; name?: string }

  let { workspace, sessions, projects, creatingProject, error, onCreateSession, onCreateProject }: {
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

<section class="brand-dashboard-grid">
  <section class="brand-dashboard-card">
    <div class="brand-card-heading"><h2 class="brand-card-title">Latest Chats</h2><button class="button is-primary is-small" type="button" onclick={() => void onCreateSession()}>New chat</button></div>
    {#each sessions as session}
      <RouterLink class="brand-dashboard-row" href={`${sessionsPath()}/${encodeURIComponent(session.id)}`}><span class="brand-row-content"><span>{session.name ?? "New Chat"}</span><span class="brand-row-meta"><time datetime={session.created_at}>{createdAtLabel(session.created_at)}</time>{#if session.project !== undefined}<span aria-hidden="true">/</span><span>{session.project.name ?? "New Project"}</span>{/if}</span></span></RouterLink>
    {:else}<p class="brand-empty">No chats yet.</p>{/each}
    {#if sessions.length > 0}<RouterLink class="brand-view-all" href={sessionsPath()}>View all chats</RouterLink>{/if}
  </section>
  <section class="brand-dashboard-card">
    <div class="brand-card-heading"><h2 class="brand-card-title">Latest Projects</h2><button class="button is-primary is-small" type="button" disabled={creatingProject} onclick={() => void onCreateProject()}>New project</button></div>
    {#each projects as project}
      <RouterLink class="brand-dashboard-row" href={`${projectsPath()}/${encodeURIComponent(project.id)}`}><span class="brand-row-content"><span>{project.name ?? "New Project"}</span><time class="brand-row-meta" datetime={project.created_at}>{createdAtLabel(project.created_at)}</time></span></RouterLink>
    {:else}<p class="brand-empty">No projects yet.</p>{/each}
    {#if projects.length > 0}<RouterLink class="brand-view-all" href={projectsPath()}>View all projects</RouterLink>{/if}
  </section>
  {#if error !== ""}<p class="help is-danger brand-error" aria-live="polite">{error}</p>{/if}
</section>
