# 2. UI patterns

[Documentation index](index.md)

These patterns describe the existing page and interaction contracts. They follow
the requirements in [Design](01-design.md). Source paths below are relative to
`dyd/assets~output=source/` within the web package.

## Application shell and navigation

### Scope

Workspace and system application pages share the shadcn inset sidebar shell.
Login and full-page loading states use the separate status layout below.

### Layout

The workspace switcher stays in the sidebar header on both page families. With
system access, it offers a System administration destination after the workspace
list. Sidebar destinations have Lucide icons; view breadcrumbs and controls stay
in the compact page header. The sidebar footer presents the signed-in user name
and sign-out action in a dropdown.

### Interaction behavior

Workspace pages fetch only their own metadata for named breadcrumbs. Chat has
additional viewport and compact-header behavior described under [Chat](#chat).

### Component references

`AppFrame`, `sidebar-page`, `Navigation`, `WorkspaceSwitcher`, `SidebarUser`, and
`Breadcrumb`; navigation data and operations live in `src/app/navigation.ts`.

## Access and full-page status

### Scope

Login, session checks, full-page loading, and account/access status views.

### Layout

Login and full-page loading states use the centered muted-surface card layout.

### Interaction behavior

Preserve credential submission, return destinations, retry, no-access messaging,
logout, and the system-access entry. Authentication/access refresh behavior is
specified in [Lifecycle](04-lifecycle.md#context-and-request-ownership).

### Component references

`StatusPage`, `Form`, `Field`, `Input`, and `Button`.

## Full-page collections

### Scope

System directories and full-page workspace chat, project, group, file, note, task,
secret, type, field, and record directories. Chat-resource dialogs and membership
rosters use the scoped patterns below.

### Layout

Directories use a full-width `SearchToolbar`, aligned `RecordTable` columns, and
`PaginationFooter`. Tables retain their column layout and scroll horizontally on
narrow screens. Record directories use configured display fields as columns.

### Interaction behavior

Search runs on the server with bounded requests. Each request selects at most
50 records. Previous/Next retain cursor history and replace the current page
rather than accumulating results. Group directories describe the principal's
enabled memberships. Failed search and pagination preserve the settled page;
see [Lifecycle](04-lifecycle.md#loading-snapshots-and-refresh).

### Component references

`AdminContent`, `AdminHeader`, `SearchToolbar`, `RecordTable`, `RecordRow`,
`RecordCell`, `TableEmpty`, and `PaginationFooter`.

## Detail pages

### Scope

Full-page workspace and system resource details. Administration can include
copyable identifiers; workspace tools follow the
[resource-identity rule](01-design.md#resource-identity-and-content).

### Layout

`AdminHeader` places titles, descriptions, status badges, identifiers where
appropriate, and actions. Details use titled `DetailSection` panels and aligned
`Property` rows, with separate Markdown content. JSON/Lisp and rate limits have
dedicated displays. Secret details display metadata without revealing stored
values.

### Interaction behavior

Project creation/editing, note/task/secret editing, historical note revisions, and
record-type field management have dedicated URLs. Global providers/models and
principal availability have dedicated edit URLs. Session resource URLs open the
dialog pattern instead of replacing the conversation.

### Component references

`AdminContent`, `AdminHeader`, `DetailSection`, `Property`, `Identifier`,
`StatusBadge`, `RichText`, `CodeBlock`, and `RateLimitsView`.

## Create and edit forms

### Scope

Full-page create/edit views. Compact dialog editors have their own layout under
[Chat-resource dialogs](#chat-resource-dialogs).

### Layout

Create/edit pages share resource-specific field compositions and retain the same
fields and section ordering. `FormSection` places its heading/description beside
a fluid field grid on desktop and above the fields on narrow screens. Long
editors span the field grid, control heights are consistent, and `FormActions`
provides a full-width save/cancel footer.

### Interaction behavior

Leaving a secret replacement blank preserves the stored value. Field settings
are separate from record-type settings; record editors retain drafts across value
pages. Failed writes and revision conflicts preserve drafts as described in
[Lifecycle](04-lifecycle.md#drafts-and-conflicts).

### Component references

`Form`, `FormSection`, `Field`, resource-specific field compositions, and
`FormActions`.

## Hubs

### Scope

System section entry points, system workspace summaries, and workspace/project
overviews.

### Layout

Hubs share `NavigationCard` and `NavigationGrid`. Workspace and project overviews
are navigation-only hubs with permanent child navigation cards. They do not load
child collections or display recent-item previews.

### Interaction behavior

Creation and uploading are available on the corresponding collection pages;
project editing has its own page. A record-type overview links to Records and
Fields. System administration lists every workspace and can create an enabled or
disabled workspace without adding workspace membership. Its inventory search and
keyset pagination run on the server.

### Component references

`AdminContent`, `AdminHeader`, `NavigationCard`, and `NavigationGrid`.

## Action emphasis and confirmations

Primary emphasis follows the importance of an action within the current view.
Main collection, dialog, connection, and recovery actions retain primary emphasis
even when they open another view. This includes creation actions, Add person or
group, Sign in, and the main recovery action on an error page. Save remains
primary when a destructive change requires a subsequent confirmation.
Supporting text controls such as Close, Cancel, History, and pagination use outlined
secondary treatment. Supporting icon-only controls, including compact row actions,
use ghost styling with visible hover and keyboard-focus states.
Success-specific styling is reserved for final affirmative actions such as
Create, Save, Add, or Submit. Header actions use the default control size; compact
row actions may use smaller sizes.

Persistent deletion, revocation, disconnection, and availability or role changes
that remove access require explicit confirmation. The opening control retains its
primary or secondary emphasis and does not use destructive styling. The final
confirmation control uses destructive treatment. Ordinary draft edits, including
staged value removals with Undo, retain editing controls.

`ConfirmationDialog` presents the target, consequences, Cancel, and the final
destructive action. Its controller in `src/utils/confirmation.svelte.ts` defers
the mutation until confirmation, blocks duplicate submissions and dismissal while
pending, and retains failures for retry or cancellation. A context change or
unmount resets the confirmation, and late results cannot affect a new request.
Targets and expected revisions are captured when the confirmation opens.

Chat-resource removals use a nested `ResourceRemoval` confirmation dialog. Participant,
project-grant, and group-secret dialogs already provide final confirmations;
their final action is destructive for removals and permission reductions.

## Reference selectors

### Scope

Resource references, including project, record-type, record, and file references.

### Layout

`RecordPicker` is a button styled as a reference field that opens a modal
search/selector. Its name includes the field label and current selection, the
search icon is part of the trigger, and clearing is a separate sibling button.

### Interaction behavior

Resource-specific pickers own their API requests. Selected references are hydrated
by ID; candidate collections load only when the dialog opens or the user
searches/paginates. Selection is a draft until confirmed, cancelling preserves
the existing reference, and closing aborts pending search requests. Owned
dialog/picker components provide keyboard navigation, focus trapping, and focus
restoration.

### Component references

`RecordPicker`, `Dialog`, and resource-specific picker compositions.

### Documentation follow-up: keychain references

The previous specification described keychain references as modal fields backed
by the provider form's keychain API. Current agent/storage provider forms expose
no keychain selector; their tests explicitly cover creating providers without
selecting or fetching a keychain. Detail pages still display keychain metadata.
Reconcile the earlier selector description with the credential workflow before
using it as guidance for a provider form.

## Membership management

### Scope

Group members, bots, and secrets, and project People/Groups membership sections.
Chat participants use the separate dialog pattern below.

### Layout

Section-based rosters use integrated search, lists, section actions, and
pagination. Project membership has separate People and Groups sections.
Group members use a table with separate Name, Role, and Actions columns. The
Actions column contains Edit and Remove icons and is shown to group managers.
Edit opens a role editor; Remove opens a confirmation. Bot rows use the Remove
icon and confirmation. Role drafts and their expected revisions remain fixed
across background refreshes until explicitly reloaded.

### Interaction behavior

Project admins can add recipients, edit roles, and remove grants; other project
members can view the roster. Workspace group admins manage group details,
people, bots, and secrets. Workspace admins can create groups; system
administrators also have the system group-management area.

Workspace group dialogs use URL query views: `?view=create` on the directory,
and `?view=details`, `members`, `bots`, `secret-create`, or
`secret-edit&secret=<id>` on a group. Dialog forms place Cancel and submission
controls in the stationary footer. The directory retains its search and cursor
history while navigating to group details and back within the workspace.

Domain roles, admission, API behavior, and permission rules are documented in
[Project access](../../../gatehouse/dyd/docs/project-access.md),
[Group administration](../../../gatehouse/dyd/docs/group-administration.md), and
[Group secrets](../../../gatehouse/dyd/docs/group-secrets.md).

### Component references

`ManagementLayout`, `ManagementSection`, `RecordList`, `SearchToolbar`,
`PaginationFooter`, `ActionButton`, and `ActionFeedback`.

## Chat

### Scope

The conversation, message feed, activity, composer, attachments, and chat tools.

### Layout

The shell is bounded to the viewport, with scrolling contained to its message
feed so the header and composer remain stationary. Jump to latest is an
arrow-down icon button anchored at the bottom right of the feed viewport, just
above the input section, outside the scrolling message content.

The viewport-contained feed sits above a stationary, full-width card-surface
bottom input section, separated by a single top divider. The larger writing area,
attachments, and action toolbar share that section with no enclosing input
border, rounded input box, shadow, floating overlay, or textarea focus ring. The
writing area starts at 6rem tall and grows to a bounded height. The composer has
no instructional footer text.

Attachments and the icon-only delivery selector sit at the left; a standard icon
send button at the right shows upload/submission progress. Selected-file cards
show truncated filenames, readable sizes, upload/error states, and separate remove
actions in a height-bounded scrolling area.

User and agent message bubbles share a 28rem minimum and 42rem maximum width,
both clamped to the available width on narrow screens.

The chat header has standard-sized desktop icon controls for Files, Notes, Tasks,
Secrets, and Participants, with accessible labels and tooltips. Rename lives in
the overflow menu. Below 768px, one Chat tools dropdown contains labelled resource
entries, Participants, and Rename. At narrow widths, ancestor breadcrumbs hide
and the chat name truncates as needed.

### Interaction behavior

Preserve Markdown and math, code/message copying, attachments, group/direct
delivery, agent mentions, keyboard send, auto-follow, and jump-to-latest.
Chat activity includes tool/thinking progress, elapsed durations, cancellation,
failures, expandable history, and approve/reject actions.

The conversation remains mounted during resource navigation; see
[Lifecycle](04-lifecycle.md#chat-and-resource-navigation) for state retention.

### Component references

`SessionWorkspace`, `SessionChatRoute`, `Feed`, `Message`, `InputBar`,
`InputBarAttachments`, `InputAttachment`, `InputRecipient`, `FloatingAction`, and
`ViewNavigation`.

## Chat-resource dialogs

### Scope

Session files, notes, tasks, and secrets, including collections, details,
creation/editing, note history/revisions, and styled removal confirmations.

### Layout

Dialogs are content-sized and viewport-bounded. Collections and note history use
the same compact width as Participants; readers have more space, and note editors
can use a wider layout. Headers and compact action footers remain
stationary around one scrolling body. Dialog dimensions change immediately between
views, without resizing or zoom animations. The chat-resource shell uses consistent
compact horizontal gutters, without additional row insets, and separators between
header, body, and footer. Body
content has vertical padding above and below it, clear of both separators.

Collections and note history have visible headers such as “Search notes” and
“Search files”. Search and any associated filters stay together at the top of the
dialog body, directly above the results. Entries share a
leading resource icon, a prominent name above secondary metadata, and trailing
badges or actions. Spacing and full-row hover/selection states distinguish entries
without individual outlines. Header controls and rows follow the same compact
gutters. Linked entries are complete link targets. There are no redundant collection
labels, table headings, shared row rules, enclosing result frames, horizontal table
scrolling, or result counts.
Pagination uses outlined controls only when more than one page is available.

Search uses a bordered field with a decorative, non-focusable icon inside it, alongside
its placeholder. The field's focus indicator encloses both icon and text, and
Enter submits the search. Progress replaces the icon without changing the field's
dimensions or adding a separate control.
Searches preserve existing results until the response arrives. Background refresh
does not insert a diagnostic paragraph or disable search controls. Collection loading
and empty results are presented within the results area.

Note readers use the displayed note's name as the dialog header. The body begins
with its optional description and a compact author/date line with sensitivity
when relevant, followed by the rendered content at normal reading size. The body
does not repeat the title or display revision metadata.
Task readers use the task title in the dialog header, with status and description
in the body and quiet provenance after the content. Secret readers use the secret
description as the dialog header, with write-only guidance and provenance in the
body. The resource owner supplies the header title explicitly; `ResourceArticle`
renders only the description, metadata, and content supplied to it. These views use
document content rather than divided property grids.

Creation actions remain primary beside the outlined Close control in the footer.
Details keep Edit as the primary action; History and removal entry points are
outlined secondary actions. Compact editors use Cancel/Save without nested form cards or
explanatory section headings. The dialog header identifies the operation, such as
“New note” or “Edit note”; editors do not repeat that heading in the body.
Write-only secret guidance remains explicit.
Footer submit buttons retain their native form association.
Each view has one dismissal control: Close for readers and collections, or Cancel
for editors and confirmations. These return to the parent view. Dialog
headers have neither a back arrow nor an X icon.

### Interaction behavior

All resource views stay within the dialog. Existing URLs support direct opening
and browser Back/Forward. Closing readers or cancelling creation returns to the
resource list; cancelling an edit returns to the item being edited. Close, Cancel,
Escape, and outside clicks use the same destination. Note history and historical
readers return to the current note; collections dismiss to the conversation.
Each resource owner composes one dialog for its search/reader/editor views;
returning to a parent resource view keeps that dialog open. Pending writes block
dismissal. Removal opens a separate nested confirmation while the reader remains
mounted. The parent cannot dismiss while that confirmation is active. Cancel,
Escape, and outside clicks close only the confirmation and restore focus to its
Remove trigger. Confirmations retain their opening item name and show failed
mutations without dismissing the confirmation.

The logical route and displayed resource have separate lifetimes. The URL returns
to the conversation immediately on dismissal, while the last resource route,
dimensions, content, and actions remain mounted and inactive through the exit
animation. `onOpenChangeComplete` releases that view after dismissal. Reopening
cannot be cleared by an obsolete completion callback; changing session, principal,
or access releases retained content immediately. Resource owners accept an
explicit displayed route and do not reinterpret the conversation URL while exiting.
Closing suspends their request ownership immediately without resetting the visible
snapshot, preventing late requests or mutations from navigating back into a
dismissed resource.

See [Lifecycle](04-lifecycle.md#chat-and-resource-navigation) for retained conversation
and collection state, focus movement, and scroll restoration. Shared request
ownership through `ResourcePage` is described in
[Architecture](03-architecture.md#resource-ownership-and-transport).

### Component references

`ResourcePage`, `ResourceDialog`, `ResourceRemoval`, `ResourceArticle`, `RecordList`,
`SearchToolbar`, and `FormActions`. Resource owners pass the dialog title, search
and footer snippets, pending state, and dismissal callback as explicit props.
`ResourceArticle`, `ResourceActions`, and `FormActions` are presentational; they do
not register content or behavior with a containing dialog.

## Participant dialogs

### Scope

The chat toolbar's Participants dialog, its People/Groups/Bots tabs, and its add, role,
and removal dialogs.

### Layout

Participant collections and add dialogs use the compact resource-dialog shell,
with visible headers, body search controls, and a stationary footer. Their unboxed `RecordList`
entries share leading type icons, name styling, spacing, and full-row
hover/selection treatment with chat resources. Content scrolls in the dialog body
rather than inside a separately capped roster.
Existing participants have role labels and icon actions; add dialogs have
selection markers. There is no explanatory boilerplate or result count, and
pagination controls appear only when needed. Add actions live in the footer.
The Participants body starts with People/Groups/Bots tabs. Each tab panel owns its
search field above its results and retains its own query and pagination state.
Participant dialogs also use footer dismissal controls without a header X icon.

### Interaction behavior

Edit opens a role dialog; Remove opens a confirmation dialog. Failed mutations
retain their dialog drafts and expected revisions across background refreshes;
cancelling aborts pending work. Grant changes refresh open rosters and
chat-directory visibility, and confirmed revocation leaves the affected chat
without resetting drafts during ordinary refreshes. See
[Lifecycle](04-lifecycle.md#chat-and-resource-navigation).

### Component references

`SessionParticipantsDialog`, the add/action dialog compositions, `Tabs`,
`RecordList`, and `ResourceDialog`.

## Generated input forms

### Scope

Shared input-request forms embedded in the conversation.

### Layout

Input-request forms use the available width, compact sections, and a consistent
submit/cancel footer.

### Interaction behavior

Forms support recursive object/list fields; required/optional text, number,
options, boolean, files, and external custom tools; shared drafts, clearing,
saving, submission, cancellation, and focus refresh. Optimistic answers, retries,
and terminal submission are specified in
[Lifecycle](04-lifecycle.md#shared-input-forms).

### Component references

`InputRequestCard`, `InputForm`, `FormField`, `CustomField`, and `InputControl`.

## Capability-based file tools

### Scope

The standalone session file-picker tool.

### Layout

File tools use the full page width, selected-file summaries, filter toolbars,
selectable tables, pagination, and a Save selection footer.

### Interaction behavior

Preserve capability-gated listing/upload/download, selected file chips, limits,
name/media filters, pagination, saved selection, and expired/denied/resolved
states. Capability transport and shared-draft completion remain unchanged.

### Component references

`ToolPage`, selected-file summaries, `RecordTable`, and `PaginationFooter`.

## Page workflows

This inventory maps page families to their feature-specific behavior. Shared
layout and lifecycle rules are owned by the sections above and by
[Lifecycle](04-lifecycle.md).

| Page family | Purpose and preserved behavior |
| --- | --- |
| Access/login | Session checks, credential submission, return destination, retry, no-access messaging, logout, and system access entry. |
| Workspace overview | Workspace description, child navigation cards, workspace/access activity refresh, and workspace selection. |
| Chat collection | Name search in the URL, results, pagination, chat creation, and project metadata. |
| Project collection | Name search in the URL, results, pagination, and project creation. |
| Groups | Search enabled memberships; create groups where permitted; inspect and manage group details, people, bots, admins, and secrets according to role. |
| Project overview | Project metadata, Edit project destination, child navigation cards, and project-metadata activity refresh. |
| Project members | Separate searchable, paginated People/Groups rosters; add recipients, edit roles, and remove grants where permitted; read-only access for other members. |
| Project files | Upload multiple files, inspect metadata, download, remove, refresh, and collection/detail navigation. |
| Session files | Search, inspect, and download human- and agent-generated chat files in one list, with chat navigation and rename. |
| Project notes | Create/edit/delete notes, sensitive content, Markdown, author metadata, revision history, and historical viewing. |
| Session notes | Create/edit/delete notes, sensitivity, Markdown, history, and dedicated edit/revision routes. |
| Tasks | Project/session collections and details, create/edit/delete, workflow status, sensitivity, Markdown, and authorship. |
| Project/session secrets | Create/edit/delete descriptions and secret values; metadata-only detail views; blank replacement preserves the current value. |
| Record types | Define/edit/delete schemas, field types, reference targets, cardinality, uniqueness, display order, and descriptions. |
| Records | Schema-generated editing, many-valued/sensitive fields, file/record references, primary/secondary display, reverse references, and pagination. |
| Chat | Markdown and math, code/message copying, attachments, group/direct delivery, agent mentions, keyboard send, auto-follow, and jump-to-latest. |
| Chat activity | Tool/thinking progress, elapsed durations, cancellation, failures, expandable history, and approve/reject actions. |
| Shared input forms | Recursive object/list fields; required/optional text, number, options, boolean, files, and external custom tools; shared drafts, clearing, saving, submission, cancellation, and focus refresh. |
| Session file-picker tool | Capability-gated listing/upload/download, selected file chips, limits, name/media filters, pagination, saved selection, and expired/denied/resolved states. |
| System overview | Entry points to all system administration areas. |
| Principals | Inspect principals and identities, revisions, enable/disable actions, and in-flight state. |
| System grants | Grant/revoke system-admin access, revisions, and activity refresh. |
| Agent providers | Search, create, inspect, and edit protocols, endpoints, credentials, enabled state, and revisions; inspect keychain metadata. See the keychain-selector follow-up above. |
| Agent models | Search, create, inspect, and edit provider/model selection, parameters, compaction, turn/token limits, enabled state, and revisions. |
| Storage providers | Search, create, inspect, and edit embedded/S3 settings, credentials, enabled state, and revisions. |
| Workspaces | Search and create workspaces; inspect a summary and edit name/status on a separate route. |
| Workspace principals | Per-workspace direct-grant roster, search, server pagination, revocation, and separate add/role-assignment flow. |
| Workspace agent bindings | Per-workspace, server-paginated list; separate create, inspect, and edit routes for model assignment, default replies, labels, prompts, Lisp preludes, token rate limits, and enabled state. |
| Workspace storage bindings | Per-workspace, server-paginated list; separate create, inspect, and edit routes for provider assignment, priority, enabled state, revisions, and revision-conflict reload. |
