<script lang="ts">
  import { PageBody } from "../../ui"
  import * as UI from "../../ui"
  import type { Snippet } from "svelte"
  import type { Route } from "../../route"
  import { createSessionSecretsController } from "./session-secrets-controller.svelte"

  let {
    workspaceID,
    sessionID,
    route,
    controller,
    actions,
    embedded = false,
  }: {
    workspaceID: string
    sessionID: string
    route: Route
    controller: ReturnType<typeof createSessionSecretsController>
    actions: Snippet
    embedded?: boolean
  } = $props()

  const secretsPath = $derived(
    `/app/wsp/${encodeURIComponent(workspaceID)}/ses/${encodeURIComponent(sessionID)}/secrets`,
  )
  function secretPath(secretID: string) {
    return `${secretsPath}/${encodeURIComponent(secretID)}`
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
  {#if !embedded && controller.state.reloading}<UI.Text role="status"
      >Refreshing secrets…</UI.Text
    >{/if}
  {#if controller.state.reloadError}<UI.Notice
      variant="destructive"
      role="alert"
      >{controller.state.reloadError}<UI.Button
        variant="outline"
        onclick={() => void controller.reload()}>Retry</UI.Button
      ></UI.Notice
    >{/if}
  {#if "secretID" in route && controller.state.detailStatus !== "ready"}
    {#if !embedded}<UI.AdminHeader title="Secret" />{/if}
    {#if controller.state.detailStatus === "checking"}<UI.Text role="status"
        >Loading secret…</UI.Text
      >{:else}<UI.Notice variant="destructive" role="alert"
        >{controller.state.error}</UI.Notice
      ><UI.Button
        variant="outline"
        onclick={() => void controller.reload()}>Try again</UI.Button
      >{/if}
  {:else if controller.state.editing}
    {#if !embedded}<UI.AdminHeader
      title={controller.state.creating ? "New secret" : "Edit secret"}
      description="Update the description and optionally replace the write-only value."
    />{/if}
    <UI.Form
      id="session-secret-form"
      surface={!embedded}
      compact={embedded}
      onsubmit={(event) => {
        event.preventDefault()
        void controller.save()
      }}
    >
      <UI.FormSection
        dialog={embedded}
        title="Description"
        description="Identify the secret without including its value."
        ><UI.Field>
          <UI.Label for="session-secret-description">Description</UI.Label>
          <UI.Textarea
            id="session-secret-description"
            autocomplete="off"
            rows={3}
            maxlength={4096}
            required
            bind:value={controller.state.description}
          ></UI.Textarea>
        </UI.Field>
      </UI.FormSection><UI.FormSection
        dialog={embedded}
        title="Secret value"
        description={controller.state.creating
          ? "Values are write-only and cannot be displayed later."
          : "Leave the replacement blank to keep the current value."}
        ><UI.Field>
          <UI.Label for="session-secret-value"
            >{controller.state.creating
              ? "Value"
              : "New value (optional)"}</UI.Label
          >
          <UI.Textarea
            id="session-secret-value"
            autocomplete="new-password"
            rows={5}
            maxlength={1048576}
            required={controller.state.creating}
            bind:value={controller.state.value}
          ></UI.Textarea>
          <UI.Text as="p" size="small">
            {controller.state.creating ? "Values are write-only and cannot be displayed later." : "Leave blank to keep the current value."}
          </UI.Text>
        </UI.Field>
      </UI.FormSection>{#if controller.state.error !== ""}<UI.Text
          as="p"
          size="small"
          role="alert"
          aria-live="polite"
        >
          {controller.state.error}
        </UI.Text>{/if}
      {#if !embedded}<UI.FormActions>{@render actions()}</UI.FormActions>{/if}
    </UI.Form>
  {:else if controller.state.active !== null}
    {@const secret = controller.state.active}
    {#if embedded}
      <UI.ResourceArticle>
        <UI.Text as="p" tone="muted">Secret values are write-only. Edit this secret to replace its stored value.</UI.Text>
        {#snippet provenance()}
          <UI.Text as="p" size="small">Created {createdAtLabel(secret.created_at)} · {secret.author.name ?? "User"}</UI.Text>
          <UI.Text as="p" size="small">Updated {createdAtLabel(secret.updated_at)}</UI.Text>
        {/snippet}
      </UI.ResourceArticle>
      {#if controller.state.error}<UI.Notice variant="destructive" role="alert">{controller.state.error}</UI.Notice>{/if}
    {:else}
    <UI.Stack as="article">
      <UI.AdminHeader
        kind="Chat secret"
        title="Secret"
        {actions}
      />
      <UI.DetailSection title="Secret information">
        <UI.Property label="Description"
          >{controller.state.active.description}</UI.Property
        >
        <UI.Property label="Author"
          >{controller.state.active.author.name ?? "User"}</UI.Property
        >
        <UI.Property label="Created"
          >{createdAtLabel(controller.state.active.created_at)}</UI.Property
        >
        <UI.Property label="Updated"
          >{createdAtLabel(controller.state.active.updated_at)}</UI.Property
        >
        <UI.Property label="Value"
          >Write-only; edit to replace the stored value.</UI.Property
        >
      </UI.DetailSection>
      {#if controller.state.error !== ""}<UI.Text
          as="p"
          size="small"
          role="alert"
          aria-live="polite"
        >
          {controller.state.error}
        </UI.Text>{/if}
    </UI.Stack>
    {/if}
  {:else}
    {#if !embedded}
      <UI.AdminHeader
        title="Secrets"
        description="Manage descriptions and write-only secret values."
        {actions}
      />
    {/if}
    {#snippet collectionSearch()}
      <UI.SearchToolbar framed={false} label="Search secrets" placeholder="Search" bind:value={controller.page.state.search} loading={controller.page.state.busy || controller.state.status === "checking"} onsearch={() => void controller.page.search(controller.reload)} />
    {/snippet}
    <UI.RecordList
      plain={embedded}
      icon="secret"
      label="Secrets"
      toolbar={embedded ? undefined : collectionSearch}
      items={controller.state.secrets.map((secret) => ({ ...secret, label: secret.description }))}
      href={(secret) => secretPath(secret.id)}
      loading={controller.page.state.busy || controller.state.busy}
      scrollable={false}
      emptyMessage={controller.state.status === "checking" ? "Loading…" : controller.state.status === "unavailable" ? "Secrets could not be loaded." : "No matching secrets."}
    >
      {#snippet details(secret)}{secret.author.name ?? "User"} · {createdAtLabel(secret.updated_at)}{/snippet}
    </UI.RecordList>
    {#if controller.page.state.history.length > 0 || controller.page.state.next}<UI.PaginationFooter
      showCount={false}
      count={controller.state.secrets.length}
      previous={controller.page.state.history.length > 0}
      next={!!controller.page.state.next}
      loading={controller.page.state.busy ||
        controller.state.status === "checking"}
      onprevious={() => void controller.page.previous(controller.reload)}
      onnext={() => void controller.page.next(controller.reload)}
    />{/if}
    {#if controller.state.status === "unavailable"}<UI.Button
        variant="outline"
        onclick={() => void controller.reload()}>Try again</UI.Button
      >{/if}
  {/if}
</PageBody>
