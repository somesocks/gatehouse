<script lang="ts">
  import * as UI from "../ui"
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
  <UI.Stack as="header"
    ><UI.Text as="p" size="eyebrow">Session file picker</UI.Text><UI.Heading
      level={1}>{picker?.state.label ?? "Select session files"}</UI.Heading
    ></UI.Stack
  >
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
        {#if picker.state.error}<UI.Text as="p" role="alert"
            >{picker.state.error}</UI.Text
          >{/if}
        {#if picker.state.saved}<UI.Text as="p" role="status"
            >Selection saved. Return to the form in chat to submit it.</UI.Text
          >{/if}
        <UI.Row justify="end">
          <UI.Button
            variant="outline"
            type="button"
            disabled={picker.state.busy}
            onclick={cancel}>Cancel</UI.Button
          >
          <UI.Button
            variant="default"
            type="button"
            disabled={picker.state.busy}
            onclick={() => void saveAndClose()}
            >{picker.state.busy ? "Working…" : "Save"}</UI.Button
          >
        </UI.Row>
      </UI.Stack>

      {#if picker.state.capabilities.includes("session.file.list")}
        <UI.Stack as="section" aria-labelledby="available-files-heading">
          <UI.Heading level={2} id="available-files-heading"
            >Available files</UI.Heading
          >
          <UI.Form
            onsubmit={(event) => {
              event.preventDefault()
              navigate({
                name: picker.state.filterName.trim(),
                mediaType: picker.state.filterMediaType.trim(),
                limit: picker.state.browse.limit,
                cursor: "",
                direction: "next",
              })
            }}
          >
            <UI.Field>
              <UI.Label for="search">Find files by name</UI.Label>
              <InputControl
                ><UI.Input
                  id="search"
                  type="search"
                  bind:value={picker.state.filterName}
                  disabled={picker.state.busy}
                /></InputControl
              >
            </UI.Field>
            <UI.Field>
              <UI.Label for="media-type">Media type</UI.Label>
              <InputControl
                ><UI.Input
                  id="media-type"
                  type="text"
                  placeholder="Any type, or image/*"
                  bind:value={picker.state.filterMediaType}
                  disabled={picker.state.busy}
                /></InputControl
              >
            </UI.Field>
            <UI.Row>
              <UI.Button
                variant="outline"
                type="submit"
                disabled={picker.state.busy || picker.state.pageLoading}
                >Apply filters</UI.Button
              >
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
          </UI.Form>
          {#if picker.state.pageLoading}<UI.Text as="p" role="status"
              >Loading files…</UI.Text
            >{/if}
          <UI.FieldSet
            disabled={picker.state.busy || picker.state.pageLoading}
            aria-busy={picker.state.pageLoading}
          >
            <UI.FieldLegend hidden>Available session files</UI.FieldLegend>
            <UI.List scroll>
              {#each picker.state.files as file (file.id)}
                <UI.Item
                  interactive
                  selected={Boolean(
                    picker.state.selected.some(
                      (entry) => entry.id === file.id,
                    ) || undefined,
                  )}
                  type="button"
                  aria-pressed={picker.state.selected.some(
                    (entry) => entry.id === file.id,
                  )}
                  onclick={() => picker.select(file)}
                >
                  <UI.Text>{file.name}</UI.Text>
                  <UI.Text as="small" size="small"
                    >{file.size} bytes{file.media_type
                      ? ` · ${file.media_type}`
                      : ""}</UI.Text
                  >
                </UI.Item>
              {:else}{#if !picker.state.pageLoading}<UI.Text as="p"
                    >No matching files on this page.</UI.Text
                  >{/if}{/each}
            </UI.List>
          </UI.FieldSet>
          <UI.Row justify="end" as="nav" aria-label="File pages">
            <UI.Button
              variant="outline"
              type="button"
              disabled={!picker.state.previousCursor ||
                picker.state.pageLoading ||
                picker.state.busy}
              onclick={() =>
                navigate(pageLink(picker.state.previousCursor, "previous"))}
              >Previous</UI.Button
            >
            <UI.Button
              variant="outline"
              type="button"
              disabled={!picker.state.nextCursor ||
                picker.state.pageLoading ||
                picker.state.busy}
              onclick={() => navigate(pageLink(picker.state.nextCursor))}
              >Next</UI.Button
            >
          </UI.Row>
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
    </UI.Panel>
  {/if}
</UI.ToolPage>
