<script lang="ts">
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
</script>

<SystemFrame active="overview" title="Overview">
  <section class="system-page">
    <p class="eyebrow">System</p>
    <h2 class="title is-3">System administration</h2>
    <p class="subtitle is-6">Manage global Gatehouse state.</p>
    <RouterLink class="system-section-link" href="/app/system/principals"><span><strong>Principals</strong><small>View and enable or disable principals and their identities.</small></span></RouterLink>
    <RouterLink class="system-section-link" href="/app/system/grants"><span><strong>System grants</strong><small>Grant or revoke system-manager access.</small></span></RouterLink>
    <RouterLink class="system-section-link" href="/app/system/agent-providers"><span><strong>Agent providers</strong><small>Configure model-provider protocols and credentials.</small></span></RouterLink>
    <RouterLink class="system-section-link" href="/app/system/agent-models"><span><strong>Agent models</strong><small>Configure models available for workspace bindings.</small></span></RouterLink>
    <RouterLink class="system-section-link" href="/app/system/storage-providers"><span><strong>Storage providers</strong><small>Configure embedded or S3 storage.</small></span></RouterLink>
    <RouterLink class="system-section-link" href="/app/system/workspace-agent-bindings"><span><strong>Workspace agent bindings</strong><small>Assign agent models to workspaces.</small></span></RouterLink>
    <RouterLink class="system-section-link" href="/app/system/workspace-storage-bindings"><span><strong>Workspace storage bindings</strong><small>Assign storage providers to workspaces.</small></span></RouterLink>
  </section>
</SystemFrame>
