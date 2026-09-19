<script lang="ts">
  import { untrack } from "svelte"
  import { useRuntime } from "../../../app/runtime.svelte"
  import SystemFrame from "../../../components/SystemFrame.svelte"
  import { createPrincipalsController } from "./principals-controller.svelte"

  const runtime = useRuntime()
  const controller = untrack(() =>
    createPrincipalsController({
      onAuthenticationLost: runtime.requireLogin,
      onSystemAccessChange: runtime.access.setSystemAccess,
      principalID: () => runtime.auth.state.claims?.principal.ref.id,
    }),
  )
  $effect(() => {
    void controller.load()
  })
</script>

<SystemFrame active="principals" title="Principals">
  <section class="stack">
    <div class="stack">
      <div>
        <p class="eyebrow">System</p>
        <h2>Principals</h2>
        <p>
          Identity associations are shown without credential verifiers.
        </p>
      </div>
    </div>
    {#if controller.state.error !== ""}<p
        class="field-help"
        role="alert"
        aria-live="polite"
      >
        {controller.state.error}
      </p>{/if}
    <div class="list">
      {#each controller.state.principals as principal (principal.id)}<article
          class="list-item surface split"
          data-disabled={!principal.enabled || undefined}
        >
          <div class="stack">
            <strong>{principal.name ?? principal.alias ?? principal.id}</strong
            ><small
              >{principal.id}{principal.alias === undefined
                ? ""
                : ` / ${principal.alias}`} / revision {principal.revision}</small
            >{#if principal.identities.length > 0}<div
                class="stack muted"
              >
                {#each principal.identities as identity (identity.id)}<span
                    class:muted={!identity.enabled}
                    >{identity.key} / {identity.id} / revision {identity.revision}{identity.enabled
                      ? ""
                      : " / Disabled"}</span
                  >{/each}
              </div>{:else}<small>No identities</small>{/if}
          </div>
          <div class="cluster">
            <span
              class:muted={!principal.enabled}
              >{principal.enabled ? "Enabled" : "Disabled"}</span
            ><button
              class="small"
              type="button"
              disabled={controller.state.updatingIDs.has(principal.id)}
              onclick={() =>
                void controller.setEnabled(principal, !principal.enabled)}
              >{controller.state.updatingIDs.has(principal.id)
                ? "Saving..."
                : principal.enabled
                  ? "Disable"
                  : "Enable"}</button
            >
          </div>
        </article>{:else}<p class="muted">
          No principals are configured.
        </p>{/each}
    </div>
  </section>
</SystemFrame>
