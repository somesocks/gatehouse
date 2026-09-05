<script lang="ts">
  import type { Workspace } from "../../app/access"

  type Session = { id: string; created_at: string; name?: string; project?: { id: string; name?: string } }
  type Project = { id: string; created_at: string; name?: string }

  let { workspace, sessions, projects, creatingProject, error, onCreateSession, onCreateProject, onNavigate }: {
    workspace: Workspace
    sessions: Session[]
    projects: Project[]
    creatingProject: boolean
    error: string
    onCreateSession: () => void | Promise<void>
    onCreateProject: () => void | Promise<void>
    onNavigate: (path: string) => void
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

<section class="dashboard-grid">
  <section class="dashboard-widget dashboard-widget-wide">
    <div class="dashboard-widget-heading"><h2>Latest Chats</h2><button class="button is-primary is-small" type="button" onclick={() => void onCreateSession()}>New chat</button></div>
    {#each sessions as session}
      <a class="dashboard-row" href={`${sessionsPath()}/${encodeURIComponent(session.id)}`} onclick={(event) => { event.preventDefault(); onNavigate(`${sessionsPath()}/${encodeURIComponent(session.id)}`) }}><span class="dashboard-row-content"><span>{session.name ?? "New Chat"}</span><span class="dashboard-row-meta"><time datetime={session.created_at}>{createdAtLabel(session.created_at)}</time>{#if session.project !== undefined}<span aria-hidden="true">/</span><span>{session.project.name ?? "New Project"}</span>{/if}</span></span></a>
    {:else}<p class="dashboard-empty">No chats yet.</p>{/each}
    {#if sessions.length > 0}<a class="dashboard-view-all" href={sessionsPath()} onclick={(event) => { event.preventDefault(); onNavigate(sessionsPath()) }}>View all chats</a>{/if}
  </section>
  <section class="dashboard-widget dashboard-widget-wide">
    <div class="dashboard-widget-heading"><h2>Latest Projects</h2><button class="button is-primary is-small" type="button" disabled={creatingProject} onclick={() => void onCreateProject()}>New project</button></div>
    {#each projects as project}
      <a class="dashboard-row" href={`${projectsPath()}/${encodeURIComponent(project.id)}`} onclick={(event) => { event.preventDefault(); onNavigate(`${projectsPath()}/${encodeURIComponent(project.id)}`) }}><span class="dashboard-row-content"><span>{project.name ?? "New Project"}</span><time datetime={project.created_at}>{createdAtLabel(project.created_at)}</time></span></a>
    {:else}<p class="dashboard-empty">No projects yet.</p>{/each}
    {#if projects.length > 0}<a class="dashboard-view-all" href={projectsPath()} onclick={(event) => { event.preventDefault(); onNavigate(projectsPath()) }}>View all projects</a>{/if}
  </section>
  {#if error !== ""}<p class="help is-danger dashboard-error" aria-live="polite">{error}</p>{/if}
</section>
