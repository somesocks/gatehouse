<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import { createSystemGrantsController } from "./system-grants-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() =>
    createSystemGrantsController({
      activity: runtime.activity,
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
      principalID: () => runtime.auth.state.claims?.principal.ref.id,
    }),
  )
  $effect(() => {
    void controller.load()
  })
  $effect(() => controller.start())
</script>

<SystemFrame active="grants" title="System grants">
  <section class="stack">
    <div class="stack">
      <div>
        <p class="eyebrow">System</p>
        <h2>System grants</h2>
        <p>
          System managers can modify global Gatehouse state.
        </p>
      </div>
    </div>
    <form
      class="cluster"
      onsubmit={(event) => {
        event.preventDefault()
        void controller.create()
      }}
    >
      <label class="field"
        ><span>Principal ID</span><input
          autocomplete="off"
          placeholder="prn_..."
          bind:value={controller.state.principal}
        /></label
      ><button
        class="primary"
        type="submit"
        disabled={controller.state.creating}
        >{controller.state.creating ? "Granting..." : "Add manager"}</button
      >
    </form>
    {#if controller.state.error !== ""}<p
        class="field-help"
        role="alert"
        aria-live="polite"
      >
        {controller.state.error}
      </p>{/if}
    <div class="list">
      {#each controller.state.grants as grant (grant.ref.id)}<article
          class="list-item surface split"
          data-disabled={!grant.enabled || undefined}
        >
          <div class="stack">
            <strong>{grant.principal.id}</strong><small
              >{grant.ref.id} / revision {grant.revision}</small
            >
          </div>
          <div class="cluster">
            <span
              class:muted={!grant.enabled}
              >{grant.enabled ? "Enabled" : "Disabled"}</span
            ><button
              class="small"
              type="button"
              disabled={controller.state.updatingIDs.has(grant.ref.id)}
              onclick={() => void controller.setEnabled(grant, !grant.enabled)}
              >{controller.state.updatingIDs.has(grant.ref.id)
                ? "Saving..."
                : grant.enabled
                  ? "Disable"
                  : "Enable"}</button
            >
          </div>
        </article>{:else}<p class="muted">
          No system grants are configured.
        </p>{/each}
    </div>
  </section>
</SystemFrame>
