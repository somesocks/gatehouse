<script lang="ts">
  import * as UI from "../ui"
  import { formatFileSize } from "../utils/file-size"
  import { onMount, untrack } from "svelte"
  import { InputControl } from "../ui"
  import SelectedFiles from "./SelectedFiles.svelte"
  import { createFieldTransport, fieldLaunchFromFragment } from "./api"
  import {
    createSessionFilePicker,
    type SessionFile,
  } from "./controller.svelte"
  import {
    fileBrowseFromSearch,
    fileBrowseURL,
    type FileBrowse,
  } from "./browse"

  const launch = fieldLaunchFromFragment(
    window.location.hash,
    window.location.origin,
  )
  const picker =
    launch === null
      ? null
      : untrack(() =>
          createSessionFilePicker(
            createFieldTransport(launch),
            fileBrowseFromSearch(window.location.search),
          ),
        )

  onMount(() => {
    if (picker) void picker.load()
    const popstate = () => {
      if (picker)
        void picker.browse(fileBrowseFromSearch(window.location.search))
    }
    window.addEventListener("popstate", popstate)
    return () => {
      window.removeEventListener("popstate", popstate)
      picker?.dispose()
    }
  })

  function navigate(browse: FileBrowse, event?: MouseEvent): void {
    if (
      event &&
      (event.ctrlKey ||
        event.metaKey ||
        event.shiftKey ||
        event.altKey ||
        event.button !== 0)
    )
      return
    event?.preventDefault()
    if (!picker) return
    const destination = fileBrowseURL(window.location.href, browse)
    if (destination !== window.location.href)
      window.history.pushState(null, "", destination)
    void picker.browse(browse)
  }

  function pageLink(
    cursor: string,
    direction: "next" | "previous" = "next",
  ): FileBrowse {
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

<svelte:head
  ><title>Session file picker · Gatehouse</title><meta
    name="referrer"
    content="no-referrer"
  /></svelte:head
>

<UI.Theme />
<UI.ToolPage>
  <UI.AdminHeader
    kind="File selection"
    title={picker?.state.label ?? "Select session files"}
    description="Choose files for the shared form. Save the selection, then return to chat."
  />
  {#if picker === null}
    <UI.Text as="p" role="alert"
      >The tool link is invalid. Reopen it from the input form.</UI.Text
    >
  {:else if picker.state.status === "loading"}
    <UI.Text as="p" role="status"
      >Loading the field and saved selection…</UI.Text
    >
  {:else if picker.state.status === "resolved"}
    <UI.Text as="p" role="status"
      >This input is already resolved. Return to chat.</UI.Text
    >
  {:else if picker.state.status === "denied"}
    <UI.Text as="p" role="alert"
      >This tool link is unavailable or you no longer have access.</UI.Text
    >
  {:else if picker.state.status === "unavailable"}
    <UI.Text as="p" role="alert">{picker.state.error}</UI.Text><UI.Button
      variant="outline"
      type="submit"
      onclick={() => void picker.load()}>Try again</UI.Button
    >
  {:else}
    <UI.Panel aria-label="Session file picker">
      <UI.Stack as="section" aria-labelledby="selected-files-heading">
        <UI.Heading level={2} id="selected-files-heading"
          >Selected files</UI.Heading
        >
        {#if picker.state.selected.length > 0}
          <SelectedFiles
            files={picker.state.selected}
            busy={picker.state.busy}
            opening={picker.state.opening}
            onRemove={picker.remove}
            onOpen={picker.state.capabilities.includes("session.file.read")
              ? openFile
              : undefined}
          />
        {:else}
          <UI.Text as="p" size="small">No files selected.</UI.Text>
        {/if}
        {#if picker.state.maxFiles !== null}<UI.Text as="p" size="small"
            >Maximum {picker.state.maxFiles} files.</UI.Text
          >{/if}
        {#if picker.state.reloading}<UI.Text role="status"
            >Refreshing file selection…</UI.Text
          >{/if}
        {#if picker.state.error}<UI.Text as="p" role="alert"
            >{picker.state.error}</UI.Text
          ><UI.Button variant="outline" onclick={() => void picker.load()}
            >Retry</UI.Button
          >{/if}
        {#if picker.state.saved}<UI.Text as="p" role="status"
            >Selection saved. Return to the form in chat to submit it.</UI.Text
          >{/if}
      </UI.Stack>

      {#if picker.state.capabilities.includes("session.file.list")}
        <UI.Stack as="section" aria-labelledby="available-files-heading">
          <UI.Heading level={2} id="available-files-heading"
            >Available files</UI.Heading
          >
          <UI.SearchToolbar
            label="Find files by name"
            placeholder="File name"
            bind:value={picker.state.filterName}
            loading={picker.state.busy || picker.state.pageLoading}
            onsearch={() => {
              navigate({
                name: picker.state.filterName.trim(),
                mediaType: picker.state.filterMediaType.trim(),
                limit: picker.state.browse.limit,
                cursor: "",
                direction: "next",
              })
            }}
          >
            {#snippet filters()}
              <UI.Field>
                <UI.Label hidden for="media-type">Media type</UI.Label>
                <InputControl
                  ><UI.Input
                    id="media-type"
                    type="text"
                    placeholder="Media type, e.g. image/*"
                    bind:value={picker.state.filterMediaType}
                    disabled={picker.state.busy}
                  /></InputControl
                >
              </UI.Field>
              <UI.Row>
                <UI.Button
                  variant="outline"
                  type="button"
                  disabled={picker.state.busy || picker.state.pageLoading}
                  onclick={() =>
                    navigate({
                      ...fileBrowseFromSearch(""),
                      limit: picker.state.browse.limit,
                    })}>Clear filters</UI.Button
                >
              </UI.Row>
            {/snippet}
          </UI.SearchToolbar>
          {#if picker.state.pageLoading}<UI.Text as="p" role="status"
              >Loading files…</UI.Text
            >{/if}
          <UI.FieldSet
            disabled={picker.state.busy || picker.state.pageLoading}
            aria-busy={picker.state.pageLoading}
          >
            <UI.FieldLegend hidden>Available session files</UI.FieldLegend>
            <UI.RecordTable
              headers={["Name", "Size", "Media type", "Selection"]}
            >
              {#each picker.state.files as file (file.id)}<UI.RecordRow>
                  <UI.RecordCell primary>{file.name}</UI.RecordCell>
                  <UI.RecordCell>{formatFileSize(file.size)}</UI.RecordCell>
                  <UI.RecordCell>{file.media_type ?? "Unknown"}</UI.RecordCell>
                  <UI.RecordCell actions
                    ><UI.Checkbox
                      aria-label={`Select ${file.name}`}
                      checked={picker.state.selected.some(
                        (entry) => entry.id === file.id,
                      )}
                      disabled={picker.state.busy || picker.state.pageLoading}
                      onCheckedChange={() => picker.select(file)}
                    /></UI.RecordCell
                  >
                </UI.RecordRow>{:else}<UI.TableEmpty
                  columns={4}
                  message={!picker.state.filesLoaded && picker.state.error
                    ? "Files could not be loaded."
                    : picker.state.pageLoading
                      ? "Loading files…"
                      : "No matching files on this page."}
                />{/each}
            </UI.RecordTable>
          </UI.FieldSet>
          <UI.PaginationFooter
            noun="files"
            count={picker.state.files.length}
            previous={!!picker.state.previousCursor}
            next={!!picker.state.nextCursor}
            loading={picker.state.pageLoading || picker.state.busy}
            previousHref={picker.state.previousCursor
              ? fileBrowseURL(window.location.href, pageLink(picker.state.previousCursor, "previous"))
              : undefined}
            nextHref={picker.state.nextCursor
              ? fileBrowseURL(window.location.href, pageLink(picker.state.nextCursor))
              : undefined}
            onprevious={(event) =>
              navigate(pageLink(picker.state.previousCursor!, "previous"), event)}
            onnext={(event) => navigate(pageLink(picker.state.nextCursor!), event)}
          />
        </UI.Stack>
      {/if}
      {#if picker.state.capabilities.includes("session.file.upload")}
        <UI.Stack as="section" aria-labelledby="upload-files-heading">
          <UI.Heading level={2} id="upload-files-heading"
            >Upload files</UI.Heading
          >
          <UI.Field>
            <UI.Label for="upload">Choose files to upload</UI.Label>
            <UI.FileInput
              id="upload"
              type="file"
              multiple={picker.state.maxFiles !== 1}
              accept={picker.state.mediaTypes.join(",") || undefined}
              disabled={picker.state.busy}
              onchange={(event) => {
                const files = Array.from(event.currentTarget.files ?? [])
                event.currentTarget.value = ""
                if (files.length) void picker.upload(files)
              }}
            />
          </UI.Field>
        </UI.Stack>
      {/if}
      <UI.FormActions
        ><UI.Button
          variant="outline"
          disabled={picker.state.busy}
          onclick={cancel}>Cancel</UI.Button
        ><UI.Button
          disabled={picker.state.busy}
          onclick={() => void saveAndClose()}
          >{picker.state.busy ? "Working…" : "Save selection"}</UI.Button
        ></UI.FormActions
      >
    </UI.Panel>
  {/if}
</UI.ToolPage>
