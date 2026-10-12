<script lang="ts">
  import { Paperclip, X, InputControl } from "../../ui"
  import * as UI from "../../ui"
  import FormField from "./FormField.svelte"
  import CustomField from "./CustomField.svelte"
  import {
    initialListValue,
    patchDraft,
    type InputField,
    type InputFileSummary,
  } from "./form-data"

  let {
    field,
    path,
    value,
    label = field.label ?? "Value",
    controlID = `input-${path.join("-")}`,
    depth = 1,
    busy = false,
    onSet,
    onRemove,
    onUpload,
    onOpen,
  }: {
    field: InputField
    path: string[]
    value: unknown
    label?: string
    controlID?: string
    depth?: number
    busy?: boolean
    onSet: (path: string[], value: unknown) => Promise<boolean>
    onRemove: (path: string[]) => Promise<boolean>
    onUpload?: (
      path: string[],
      files: File[],
      save: (value: InputFileSummary) => Promise<boolean>,
    ) => Promise<boolean>
    onOpen?: (path: string[]) => Promise<string>
  } = $props()

  const entries = $derived(Array.isArray(value) ? value : [])
  const hasClearButton = $derived(
    field.type === "text" || field.type === "number",
  )
  const clearable = $derived(hasClearButton && value !== undefined)
  let addingEntry = $state(false)
  let savingCount = $state(0)
  const saving = $derived(savingCount > 0)
  let inputElement = $state<HTMLInputElement | undefined>()
  let clearButton = $state<HTMLButtonElement | undefined>()
  let fileElement = $state<HTMLInputElement | undefined>()
  let fileError = $state("")

  async function uploadFiles(selected: File[]): Promise<void> {
    if (selected.length === 0 || onUpload === undefined) return
    const current: InputFileSummary[] = Array.isArray(value) ? [...value] : []
    if (
      field.max_files !== undefined &&
      BigInt(current.length + selected.length) > BigInt(field.max_files)
    ) {
      fileError = `Select no more than ${field.max_files} files.`
      return
    }
    fileError = ""
    savingCount += 1
    try {
      const invalid = selected.find(
        (file) =>
          field.media_types !== undefined &&
          !field.media_types.some(
            (accepted) =>
              accepted === file.type ||
              (accepted.endsWith("/*") &&
                file.type.startsWith(accepted.slice(0, -1))),
          ),
      )
      if (invalid !== undefined) {
        fileError = `${invalid.name} has an unsupported media type.`
        return
      }
      await onUpload(path, selected, async (summary) => {
        const next = [...current, summary]
        if (!(await onSet(path, next))) return false
        current.push(summary)
        return true
      })
    } finally {
      savingCount -= 1
    }
  }

  function removeFile(index: number): void {
    const current: InputFileSummary[] = Array.isArray(value) ? value : []
    const next = current.filter((_, at) => at !== index)
    if (next.length === 0) void saveRemove(path)
    else void saveSet(path, next)
  }

  function focusNextFormControl(button: HTMLButtonElement): void {
    const controls = Array.from(button.form?.elements ?? [])
    for (const next of controls.slice(controls.indexOf(button) + 1)) {
      if (
        next instanceof HTMLElement &&
        next.tabIndex >= 0 &&
        !next.matches(":disabled") &&
        next.getClientRects().length > 0
      ) {
        next.focus()
        return
      }
    }
  }

  $effect(() => {
    if (
      (saving || busy || !clearable) &&
      clearButton === document.activeElement &&
      clearButton !== undefined
    )
      focusNextFormControl(clearButton)
  })

  function clearAnswer(): void {
    if (busy || saving || !clearable) return
    inputElement?.focus()
    void saveRemove(path)
  }

  async function saveSet(path: string[], next: unknown): Promise<boolean> {
    savingCount += 1
    try {
      return await onSet(path, next)
    } finally {
      savingCount -= 1
    }
  }

  async function saveRemove(path: string[]): Promise<boolean> {
    savingCount += 1
    try {
      return await onRemove(path)
    } finally {
      savingCount -= 1
    }
  }

  function isObject(value: unknown): value is Record<string, unknown> {
    return value !== null && typeof value === "object" && !Array.isArray(value)
  }

  function setEntry(
    index: number,
    childPath: string[],
    next: unknown,
    remove = false,
  ): Promise<boolean> {
    const segments = childPath.slice(path.length)
    if (remove && segments.length === 0)
      return onSet(
        path,
        entries.filter((_, position) => position !== index),
      )
    const item = entries[index]
    const updated =
      segments.length === 0
        ? next
        : patchDraft(isObject(item) ? item : {}, segments, next, remove)
    return onSet(
      path,
      entries.map((entry, position) => (position === index ? updated : entry)),
    )
  }
</script>

{#snippet fieldTitle()}
  {label}{#if !field.optional}
    <UI.Text aria-hidden="true">*</UI.Text><UI.Text hidden>
      required</UI.Text
    >{/if}
{/snippet}

{#snippet clearAction()}
  <UI.Button
    variant="ghost"
    size="icon"
    bind:ref={clearButton}
    type="button"
    aria-label={`Clear ${label}`}
    title={`Clear ${label}`}
    hidden={Boolean((!clearable && !saving) || undefined)}
    aria-hidden={(!clearable && !saving) || undefined}
    aria-disabled={busy || saving || undefined}
    tabindex={busy || saving || !clearable ? -1 : 0}
    onfocus={(event) => {
      if (
        (saving || busy || !clearable) &&
        event.currentTarget instanceof HTMLButtonElement
      )
        focusNextFormControl(event.currentTarget)
    }}
    onclick={clearAnswer}
    ><X size={16} strokeWidth={2} aria-hidden="true" /></UI.Button
  >
{/snippet}

{#if field.type === "object"}
  <UI.FormSection
    compact
    level={Math.min(depth + 1, 6)}
    title={`${label}${field.optional ? " (optional)" : " (required)"}`}
  >
    <UI.Field>
      {#if field.optional && value === undefined}
        <UI.Row>
          <UI.Button
            variant="outline"
            size="sm"
            type="button"
            disabled={busy}
            onclick={() => void saveSet(path, {})}>Add {label}</UI.Button
          >
          {#if saving}<UI.Spinner role="status"
              ><UI.Text hidden>Saving {label}</UI.Text></UI.Spinner
            >{/if}
        </UI.Row>
      {:else}
        {#if field.optional}
          <UI.Row>
            <UI.Button
              variant="outline"
              size="sm"
              type="button"
              disabled={busy}
              onclick={() => void saveRemove(path)}>Remove {label}</UI.Button
            >
            {#if saving}<UI.Spinner role="status"
                ><UI.Text hidden>Saving {label}</UI.Text></UI.Spinner
              >{/if}
          </UI.Row>
        {/if}
        {#each field.fields ?? [] as child (child.id)}
          <FormField
            field={child}
            path={[...path, child.id ?? ""]}
            value={isObject(value) && Object.hasOwn(value, child.id ?? "")
              ? value[child.id ?? ""]
              : undefined}
            label={child.label ?? "Value"}
            controlID={`${controlID}-${child.id}`}
            depth={depth + 1}
            {busy}
            {onSet}
            {onRemove}
            {onUpload}
            {onOpen}
          />
        {/each}
      {/if}
    </UI.Field>
  </UI.FormSection>
{:else if field.type === "list"}
  <UI.FormSection
    compact
    level={Math.min(depth + 1, 6)}
    title={`${label}${field.optional ? " (optional)" : " (required)"}`}
  >
    {#if saving}
      <UI.Spinner role="status"
        ><UI.Text hidden>Saving {label}</UI.Text></UI.Spinner
      >{/if}
    {#if field.optional && value === undefined}
      <UI.Button
        variant="outline"
        size="sm"
        type="button"
        disabled={busy}
        onclick={() => void saveSet(path, [])}>Add {label}</UI.Button
      >
    {:else}
      {#each entries as entry, index (index)}
        <UI.Panel>
          {#if field.item !== undefined}
            <FormField
              field={field.item}
              {path}
              value={entry}
              label={`Entry ${index + 1}`}
              controlID={`${controlID}-${index}`}
              {depth}
              {busy}
              {onUpload}
              onSet={(childPath, next) => setEntry(index, childPath, next)}
              onRemove={(childPath) =>
                setEntry(index, childPath, undefined, true)}
            />
          {/if}
          <UI.Button
            variant="outline"
            size="sm"
            type="button"
            disabled={busy}
            onclick={() =>
              void saveSet(
                path,
                entries.filter((_, position) => position !== index),
              )}>Remove entry {index + 1}</UI.Button
          >
        </UI.Panel>
      {/each}
      {#if addingEntry && field.item !== undefined}
        <UI.Panel>
          <FormField
            field={field.item}
            {path}
            value={undefined}
            label="New entry"
            controlID={`${controlID}-new`}
            {depth}
            {busy}
            {onUpload}
            onSet={async (_childPath, next) => {
              const saved = await onSet(path, [...entries, next])
              if (saved) addingEntry = false
              return saved
            }}
            onRemove={async () => {
              addingEntry = false
              return true
            }}
          />
          <UI.Button
            variant="outline"
            size="sm"
            type="button"
            onclick={() => (addingEntry = false)}>Discard new entry</UI.Button
          >
        </UI.Panel>
      {/if}
      <UI.Row>
        <UI.Button
          variant="outline"
          size="sm"
          type="button"
          disabled={busy || addingEntry || field.item === undefined}
          onclick={() => {
            if (field.item === undefined) return
            if (
              field.item.type === "text" ||
              field.item.type === "number" ||
              field.item.type === "options" ||
              field.item.type === "files"
            )
              addingEntry = true
            else void saveSet(path, [...entries, initialListValue(field.item)])
          }}>Add entry</UI.Button
        >
        {#if field.optional}<UI.Button
            variant="outline"
            size="sm"
            type="button"
            disabled={busy}
            onclick={() => void saveRemove(path)}>Remove {label}</UI.Button
          >{/if}
      </UI.Row>
    {/if}
  </UI.FormSection>
{:else}
  <UI.Field>
    {#if field.type === "custom"}
      <UI.Text as="p">{@render fieldTitle()}</UI.Text>
      <CustomField {path} {value} {busy} {onOpen} {onRemove} {label} />
    {:else if field.type === "files"}
      <UI.Label for={controlID}>{@render fieldTitle()}</UI.Label>
      <UI.FileInput
        hidden
        bind:ref={fileElement}
        id={controlID}
        type="file"
        tabindex={-1}
        multiple={field.max_files === undefined || BigInt(field.max_files) > 1n}
        accept={field.media_types?.join(",")}
        disabled={busy || saving}
        onchange={(event) => {
          const files = Array.from(event.currentTarget.files ?? [])
          event.currentTarget.value = ""
          void uploadFiles(files)
        }}
      />
      <UI.Row>
        <UI.Button
          variant="outline"
          size="sm"
          type="button"
          disabled={busy || saving || onUpload === undefined}
          onclick={() => fileElement?.click()}>Add files</UI.Button
        >
        {#if saving}<UI.Spinner role="status"
            ><UI.Text hidden>Uploading {label}</UI.Text></UI.Spinner
          >{/if}
      </UI.Row>
      {#if Array.isArray(value) && value.length > 0}
        <UI.Row aria-label={`${label} files`}>
          {#each value as file, index (file.id)}
            <UI.Chip
              ><UI.Text
                ><Paperclip
                  size={14}
                  strokeWidth={2}
                  aria-hidden="true"
                /></UI.Text
              ><UI.Row as="span"
                ><UI.Text>{file.name}</UI.Text><UI.Text as="small" size="small"
                  >{file.size} bytes</UI.Text
                ></UI.Row
              >
              <UI.Button
                variant="ghost"
                size="icon"
                type="button"
                aria-label={`Remove ${file.name}`}
                disabled={busy || saving}
                onclick={() => removeFile(index)}
                ><X size={14} strokeWidth={2} aria-hidden="true" /></UI.Button
              >
            </UI.Chip>
          {/each}
        </UI.Row>
      {/if}
      {#if fileError !== ""}<UI.Text as="p" size="small" role="alert"
          >{fileError}</UI.Text
        >{/if}
    {:else if field.type === "boolean" && !field.optional}
      <UI.Label layout="choice" for={controlID}>
        <UI.Checkbox
          id={controlID}
          checked={value === true}
          disabled={busy}
          onCheckedChange={(checked) => void saveSet(path, checked)}
        />
        {@render fieldTitle()}
        {#if saving}<UI.Spinner role="status"
            ><UI.Text hidden>Saving {label}</UI.Text></UI.Spinner
          >{/if}
      </UI.Label>
    {:else}
      <UI.Label for={controlID}>{@render fieldTitle()}</UI.Label>
      <InputControl
        loading={saving}
        loadingLabel={`Saving ${label}`}
        trailing={hasClearButton ? clearAction : undefined}
      >
        {#if field.type === "boolean"}
          <UI.Select
            id={controlID}
            disabled={busy}
            value={value === undefined ? "" : String(value)}
            onchange={(event) => {
              if (event.currentTarget.value === "") void saveRemove(path)
              else void saveSet(path, event.currentTarget.value === "true")
            }}
          >
            <UI.Option value="">Unanswered</UI.Option><UI.Option value="true"
              >Yes</UI.Option
            ><UI.Option value="false">No</UI.Option>
          </UI.Select>
        {:else if field.type === "options"}
          <UI.Select
            id={controlID}
            disabled={busy}
            value={typeof value === "string" ? value : ""}
            onchange={(event) => {
              if (event.currentTarget.value === "") void saveRemove(path)
              else void saveSet(path, event.currentTarget.value)
            }}
          >
            <UI.Option value="">Select an option</UI.Option>
            {#each field.choices ?? [] as choice (choice)}<UI.Option
                value={choice}>{choice}</UI.Option
              >{/each}
          </UI.Select>
        {:else}
          <UI.Input
            bind:ref={inputElement}
            id={controlID}
            type="text"
            inputmode={field.type === "number" ? "decimal" : undefined}
            value={typeof value === "string" ? value : ""}
            disabled={busy}
            onchange={(event) => void saveSet(path, event.currentTarget.value)}
          />
        {/if}
      </InputControl>
    {/if}
  </UI.Field>
{/if}
