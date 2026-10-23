# 4. Request and draft lifecycle

[Documentation index](index.md)

Each page owns its native resource snapshots, form drafts, and mutation state.
`src/utils/load-state.svelte.ts` supplies request ownership and cancellation;
it does not introduce a generic resource model or transport. See
[Architecture](03-architecture.md#resource-ownership-and-transport) for transport
conventions.

## Loading, snapshots, and refresh

- Before the first successful response, show loading, then an explicit failure
  and retry if the request fails. An empty successful collection is a snapshot.
- Reloading an existing context keeps its content and form controls mounted.
  Pending work and reload failures are separate indicators; transient failures
  retain the last successful data and the user's draft.
- Collection results, applied filters, continuation, and cursor history commit
  after successful requests. Failed search/Next/Previous retain the current page.
- Activity reconciliation reports failure to the poller so it can retry rather
  than acknowledging an unsuccessful refresh.

## Context and request ownership

- Authentication and access refreshes retain settled claims/access on transient
  failure. Confirmed loss of authentication or access stops the affected work.
- Principal, workspace, owner, resource, mode, and revision changes establish a
  new context; equivalent route resolution and renamed workspace metadata do not.
- Controller activation effects track their intended identity/access inputs and
  execute controller internals through `untrack`. Synchronous cleanup cancels
  requests and subscriptions.
- Request ownership is checked after decoding and before publication, errors,
  authentication callbacks, navigation, or focus.

## Drafts and conflicts

- Edit baselines are separate from refreshed snapshots. A conflicting admin write
  preserves the draft and its expected revision; replacing it with the latest
  settings requires the explicit form action.
- Record editors retain drafts across value pages.
- Removal confirmations retain their opening item name and show failed mutations
  without dismissing the confirmation.
- Participant mutations retain their dialog drafts and expected revisions across
  background refreshes; cancelling aborts pending work.

## Shared input forms

Shared input forms keep optimistic answers after a failed patch. Explicit retry
reads the authoritative draft and reapplies only unsaved field operations;
invalid answers remain editable, and terminal submission waits for uploads and
successful writes. Field removal is validated against the descriptor as well.

## Chat and resource navigation

Chat refreshes preserve composer state and keep agent branches, input cards, and
scrolling behavior independent of metadata reloads.

`SessionWorkspace` keeps one conversation mounted per session while resource
routes change inside its dialog. The conversation retains its draft, selected
attachments, scroll position, delivery mode, and activity subscription. A new
session or principal establishes a new conversation.

Closing a resource returns to the saved conversation route, including direct-agent
selection. In-dialog navigation moves focus to the dialog header and restores
each view's scroll position when returning.

Collection snapshots, search drafts, applied filters, continuation cursors, and
cursor history survive list/detail/edit navigation; abandoned page requests are
cancelled without resetting the settled page. Search and pagination changes start
the list at the top. A different owner or principal resets the resource collection.

Participant grant changes refresh open rosters and chat-directory visibility.
Confirmed revocation leaves the affected chat without resetting drafts during
ordinary refreshes.

See [chat-resource dialogs](02-patterns.md#chat-resource-dialogs) for URL, Back,
Close, and layout behavior.

## Verification

Lifecycle regressions compile the real rune modules and route activation scripts
against Svelte's client runtime with controlled promises. These checks require no
browser or DOM and cover failed reloads, access updates, conflicts, overlapping
responses, late decoding, and disposal.

Commands for running checks are in [Development](05-development.md#build-and-check).
