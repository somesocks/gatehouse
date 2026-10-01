<script lang="ts">
  import { onMount, untrack } from "svelte"
  import InputControl from "../components/InputControl.svelte"
  import SelectedFiles from "./SelectedFiles.svelte"
  import { createFieldTransport, fieldLaunchFromFragment } from "./api"
  import { createSessionFilePicker, type SessionFile } from "./controller.svelte"
  import { fileBrowseFromSearch, fileBrowseURL, type FileBrowse } from "./browse"

  const launch = fieldLaunchFromFragment(window.location.hash, window.location.origin)
  const picker = launch === null ? null : untrack(() => createSessionFilePicker(createFieldTransport(launch), fileBrowseFromSearch(window.location.search)))

  onMount(() => {
    if (picker) void picker.load()
    const popstate = () => { if (picker) void picker.browse(fileBrowseFromSearch(window.location.search)) }
    window.addEventListener("popstate", popstate)
    return () => { window.removeEventListener("popstate", popstate); picker?.dispose() }
  })

  function navigate(browse: FileBrowse, event?: MouseEvent): void {
    if (event && (event.ctrlKey || event.metaKey || event.shiftKey || event.altKey || event.button !== 0)) return
    event?.preventDefault()
    if (!picker) return
    const destination = fileBrowseURL(window.location.href, browse)
    if (destination !== window.location.href) window.history.pushState(null, "", destination)
    void picker.browse(browse)
  }

  function pageLink(cursor: string, direction: "next" | "previous" = "next"): FileBrowse {
    return { ...picker!.state.browse, cursor, direction }
  }

  function cancel(): void {
    window.close()
  }

  async function saveAndClose(): Promise<void> {
    if (!picker) return
    if (await picker.save()) window.close()
  }

  async function openFile(file: SessionFile): Promise<void> {
    const url = await picker?.download(file.id)
    if (!url) return
    // This is a short-lived single-object GET token, never the field token.
    // Native same-origin downloads avoid buffering the whole file as a Blob.
    const link = document.createElement("a")
    link.href = url
    link.download = file.name
    link.target = "_blank"
    link.rel = "noopener noreferrer"
    link.referrerPolicy = "no-referrer"
    document.body.append(link)
    link.click()
    link.remove()
  }
</script>

<svelte:head><title>Session file picker · Gatehouse</title><meta name="referrer" content="no-referrer" /></svelte:head>

<main class="input-tool-page stack">
  <header class="stack"><p class="eyebrow">Session file picker</p><h1>{picker?.state.label ?? "Select session files"}</h1></header>
  {#if picker === null}
    <p role="alert">The tool link is invalid. Reopen it from the input form.</p>
  {:else if picker.state.status === "loading"}
    <p role="status">Loading the field and saved selection…</p>
  {:else if picker.state.status === "resolved"}
    <p role="status">This input is already resolved. Return to chat.</p>
  {:else if picker.state.status === "denied"}
    <p role="alert">This tool link is unavailable or you no longer have access.</p>
  {:else if picker.state.status === "unavailable"}
    <p role="alert">{picker.state.error}</p><button class="secondary" onclick={() => void picker.load()}>Try again</button>
  {:else}
    <section class="card surface stack session-file-picker-card" aria-label="Session file picker">
      <section class="stack session-file-picker-section" aria-labelledby="selected-files-heading">
        <h2 id="selected-files-heading">Selected files</h2>
        {#if picker.state.selected.length > 0}
          <SelectedFiles files={picker.state.selected} busy={picker.state.busy} opening={picker.state.opening} onRemove={picker.remove} onOpen={picker.state.capabilities.includes("session.file.read") ? openFile : undefined} />
        {:else}
          <p class="field-help">No files selected.</p>
        {/if}
        {#if picker.state.maxFiles !== null}<p class="field-help">Maximum {picker.state.maxFiles} files.</p>{/if}
        {#if picker.state.error}<p role="alert">{picker.state.error}</p>{/if}
        {#if picker.state.saved}<p role="status">Selection saved. Return to the form in chat to submit it.</p>{/if}
        <div class="cluster session-file-picker-actions">
          <button class="secondary" type="button" disabled={picker.state.busy} onclick={cancel}>Cancel</button>
          <button class="primary" type="button" disabled={picker.state.busy} onclick={() => void saveAndClose()}>{picker.state.busy ? "Working…" : "Save"}</button>
        </div>
      </section>

      {#if picker.state.capabilities.includes("session.file.list")}
        <section class="stack session-file-picker-section" aria-labelledby="available-files-heading">
          <h2 id="available-files-heading">Available files</h2>
          <form class="stack session-file-picker-filters" onsubmit={(event) => {
            event.preventDefault()
            navigate({ name: picker.state.filterName.trim(), mediaType: picker.state.filterMediaType.trim(), limit: picker.state.browse.limit, cursor: "", direction: "next" })
          }}>
            <div class="field">
              <label for="search">Find files by name</label>
              <InputControl><input id="search" type="search" bind:value={picker.state.filterName} disabled={picker.state.busy} /></InputControl>
            </div>
            <div class="field">
              <label for="media-type">Media type</label>
              <InputControl><input id="media-type" type="text" placeholder="Any type, or image/*" bind:value={picker.state.filterMediaType} disabled={picker.state.busy} /></InputControl>
            </div>
            <div class="cluster">
              <button class="secondary" type="submit" disabled={picker.state.busy || picker.state.pageLoading}>Apply filters</button>
              <button class="secondary" type="button" disabled={picker.state.busy || picker.state.pageLoading} onclick={() => navigate({ ...fileBrowseFromSearch(""), limit: picker.state.browse.limit })}>Clear filters</button>
            </div>
          </form>
          {#if picker.state.pageLoading}<p role="status">Loading files…</p>{/if}
          <fieldset class="stack" disabled={picker.state.busy || picker.state.pageLoading} aria-busy={picker.state.pageLoading}>
            <legend class="visually-hidden">Available session files</legend>
            <div class="list session-file-picker-options">
              {#each picker.state.files as file (file.id)}
                <button class="list-item surface down session-file-picker-row" data-selected={picker.state.selected.some((entry) => entry.id === file.id) || undefined} type="button" aria-pressed={picker.state.selected.some((entry) => entry.id === file.id)} onclick={() => picker.select(file)}>
                  <span>{file.name}</span>
                  <small>{file.size} bytes{file.media_type ? ` · ${file.media_type}` : ""}</small>
                </button>
              {:else}{#if !picker.state.pageLoading}<p>No matching files on this page.</p>{/if}{/each}
            </div>
          </fieldset>
          <nav class="split session-file-picker-pagination" aria-label="File pages">
            <button class="secondary" type="button" disabled={!picker.state.previousCursor || picker.state.pageLoading || picker.state.busy} onclick={() => navigate(pageLink(picker.state.previousCursor, "previous"))}>Previous</button>
            <button class="secondary" type="button" disabled={!picker.state.nextCursor || picker.state.pageLoading || picker.state.busy} onclick={() => navigate(pageLink(picker.state.nextCursor))}>Next</button>
          </nav>
        </section>
      {/if}
      {#if picker.state.capabilities.includes("session.file.upload")}
        <section class="stack session-file-picker-section" aria-labelledby="upload-files-heading">
          <h2 id="upload-files-heading">Upload files</h2>
          <div class="field">
            <label for="upload">Choose files to upload</label>
            <input id="upload" type="file" multiple={picker.state.maxFiles !== 1} accept={picker.state.mediaTypes.join(",") || undefined} disabled={picker.state.busy}
              onchange={(event) => { const files = Array.from(event.currentTarget.files ?? []); event.currentTarget.value = ""; if (files.length) void picker.upload(files) }} />
          </div>
        </section>
      {/if}
    </section>
  {/if}
</main>

<style>
  .session-file-picker-card { inline-size: min(100%, 48rem); margin-inline: auto; }
  .session-file-picker-section { padding-block-start: 0; }
  .session-file-picker-filters { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 15rem), 1fr)); align-items: end; }
  .session-file-picker-options { max-block-size: min(50vh, 32rem); overflow: auto; }
  .session-file-picker-row { align-items: center; border: 1px solid var(--border); display: flex; gap: var(--space); justify-content: space-between; inline-size: 100%; text-align: start; }
  .session-file-picker-row small { color: var(--foreground-muted); font-size: var(--small-size); }
  .session-file-picker-row[data-selected] { border-color: var(--primary-500); }
  .session-file-picker-pagination { align-items: center; gap: calc(var(--space) / 2); justify-content: flex-end; }
  .session-file-picker-actions { justify-content: flex-end; }
</style>
