<script lang="ts">
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import RouterLink from "../../../components/RouterLink.svelte"
</script>

<SystemFrame active="overview" title="Overview">
  <section class="stack">
    <hgroup>
      <p class="eyebrow">System</p>
      <h2>System administration</h2>
      <p>Manage global Gatehouse state.</p>
    </hgroup>
    <RouterLink class="list-item surface stack" href="/app/system/principals"
      ><span class="stack"
        ><strong>Principals</strong><small
          >View and enable or disable principals and their identities.</small
        ></span
      ></RouterLink
    >
    <RouterLink class="list-item surface stack" href="/app/system/grants"
      ><span class="stack"
        ><strong>System grants</strong><small
          >Grant or revoke system-manager access.</small
        ></span
      ></RouterLink
    >
    <RouterLink class="list-item surface stack" href="/app/system/agent-providers"
      ><span class="stack"
        ><strong>Agent providers</strong><small
          >Configure model-provider protocols and credentials.</small
        ></span
      ></RouterLink
    >
    <RouterLink class="list-item surface stack" href="/app/system/agent-models"
      ><span class="stack"
        ><strong>Agent models</strong><small
          >Configure models available for workspace bindings.</small
        ></span
      ></RouterLink
    >
    <RouterLink class="list-item surface stack" href="/app/system/storage-providers"
      ><span class="stack"
        ><strong>Storage providers</strong><small
          >Configure embedded or S3 storage.</small
        ></span
      ></RouterLink
    >
    <RouterLink
      class="list-item surface stack"
      href="/app/system/workspace-agent-bindings"
      ><span class="stack"
        ><strong>Workspace agent bindings</strong><small
          >Assign agent models to workspaces.</small
        ></span
      ></RouterLink
    >
    <RouterLink
      class="list-item surface stack"
      href="/app/system/workspace-storage-bindings"
      ><span class="stack"
        ><strong>Workspace storage bindings</strong><small
          >Assign storage providers to workspaces.</small
        ></span
      ></RouterLink
    >
  </section>
</SystemFrame>
