# 1. UI design

[Documentation index](index.md)

This section defines UI design requirements. Page and interaction details are in
[Patterns](02-patterns.md), and implementation conventions are in
[Architecture](03-architecture.md).

## Layout and device support

The application supports both mobile and desktop layouts. Core workflows,
including navigation, creation, editing, search, uploads, and dialogs, remain
usable on both.

Workspace and system pages use the full available content width for collections,
details, forms, and hubs.

Pages use consistent outer gutters. Layouts adapt to the space available without
shrinking text or controls to fit a desktop composition. Related headings,
toolbars, content, and action regions share alignment edges. Multi-column layouts
stack in a logical reading order when space is limited.

- **Navigation:** Mobile users can identify their current context and reach the
  same destinations and actions as desktop users. Controls may move into labelled
  menus.
- **Forms:** Labels, guidance, fields, and validation remain readable. Fields and
  action groups stack when necessary.
- **Touch interaction:** Controls have adequate touch targets and spacing.
  Essential actions and information are available without hover.
- **Overflow:** The page does not scroll horizontally. Wide tables and code may
  scroll horizontally within a clearly bounded region, preserving access to
  their content.
- **Dialogs:** Dialogs fit the available viewport. Their content scrolls, and
  navigation, dismissal, and submission controls remain reachable.
- **On-screen keyboard:** Opening the keyboard does not make the active field or
  the controls needed to complete the task inaccessible.
- **Content resilience:** Long names, filenames, validation messages, and
  translated or enlarged text do not overlap controls or break the layout.

Shared layouts are reviewed at narrow mobile and wide desktop sizes,
including intermediate widths. Interactive mobile workflows are also checked
with the on-screen keyboard open.

## Shared page structure

Pages serving the same purpose use consistent layouts and presentation
conventions.

Common elements—including headings, descriptions, status indicators, and
actions—have consistent placement and treatment.

Create and edit views for the same resource retain consistent field grouping
and order.

## Resource identity and content

Resources are identified by human-readable names, descriptions, and meaningful
content.

Workspace interfaces do not display internal IDs, including in resource details,
collections, selectors, file summaries, and authorship labels. Administrative
interfaces may display copyable IDs.

Resource details present identity, metadata, and substantive content distinctly
and consistently.

## Pages and dialogs

Pages provide the primary context for viewing and managing information.

Dialogs support tasks within an existing context. Use them when retaining the
underlying view helps the user complete the task.

The underlying view's state is preserved when a dialog opens or closes. Closing
returns the user to that context.

Dialogs clearly identify their task and provide an obvious way to close or
cancel.

## Navigation and URLs

Navigation clearly communicates the current location, the selected destination,
and the relationship between views.

Navigable views have stable URLs that support direct linking, opening in another
tab, and reloading. The URL identifies the resource and view being shown,
including editing modes and dialogs that participate in navigation.

Browser Back and Forward restore the corresponding views. In-application
navigation behaves consistently with browser navigation.

Navigation uses links to preserve standard browser interactions.

Temporary interaction states, such as open menus and tooltips, do not require
separate URLs.

## Visual conventions

Typography, colors, spacing, borders, corner radii, and control sizes use a
shared set of styles.

Elements with the same purpose receive the same visual treatment. Headings, body
content, supporting information, actions, and status indicators are clearly
distinguishable.

Color and emphasis communicate consistent meanings throughout the application.
Selected, focused, hovered, disabled, and pending states are distinguishable.

Surfaces, borders, and dividers communicate grouping and hierarchy consistently.

The UI supports light and dark themes using the shared theme tokens. Workspace
theme overrides preserve readability and the distinction between interactive
states.

The default theme follows the system color scheme. System and access pages use
the default theme.

## Actions

Visual emphasis distinguishes primary, secondary, and destructive actions.
Actions with equivalent importance and consequences receive equivalent treatment.

Primary emphasis identifies the main action in a view, including actions that
open an editor. Secondary buttons retain visible outlines so that their interactive
boundaries are clear. Primary emphasis is distinct from success-specific styling,
which is reserved for final
affirmative actions, including form submissions.

Destructive operations require explicit confirmation before they are committed.
The confirmation identifies what will be affected and explains the relevant
consequences. Destructive treatment is reserved for the final confirming action.
Cancelling a confirmation leaves the affected data and state unchanged.

Local, undoable draft changes use ordinary editing controls. Destructive changes
that save immediately require confirmation even when initiated through a form.

Each action has a clear scope: users can identify what it affects.

While an action is pending, the interface communicates progress and prevents
duplicate submission. Controls remain stable so that progress indicators and
label changes do not shift the layout.

Icon-only actions have accessible names and an understandable purpose. Essential
actions remain discoverable on both mobile and desktop.

## Feedback

The interface clearly distinguishes loading, empty results, unavailable content,
and failed operations.

Feedback is clearly associated with the affected content or action. Messages
explain what happened and, where possible, how to proceed.

The outcome of a successful operation is apparent through updated content or
explicit confirmation.

Failed operations preserve entered values and provide a clear way to recover or
retry where applicable.

Background refresh preserves usable content and the user's work. Refresh status
and failures remain distinguishable from initial loading.

Feedback is proportionate to its importance. The same event does not produce
redundant messages.

## Forms

Forms group related fields and present them in a logical, consistent order.

Each field has a clear label. Placeholder text does not replace the label.
Guidance explains relevant constraints, expected values, or consequences.

Required and optional fields are indicated consistently, making clear what is
needed to complete the form.

Input controls suit the information being entered and support normal browser
input behavior, including autofill where appropriate.

Validation identifies the affected fields and explains how to correct their
values. Entered values remain available for correction.

Users can distinguish changes that save automatically from those that require
submission, and can tell whether their changes have been saved.

Cancellation has a predictable effect, and the interface makes clear whether
changes will be discarded or have already been saved.

## Accessibility

Core workflows are usable with a keyboard and assistive technology, as well as
pointer and touch input.

Keyboard focus is visible and follows a logical order. Focus moves predictably
when views open or close, and users can leave controls and dialogs without
becoming trapped.

Headings, labels, controls, and their states are exposed meaningfully to assistive
technology. Reading order remains coherent across layouts.

Text and interactive elements have sufficient contrast. Color alone does not
communicate meaning, status, or selection.

Content and controls remain usable when text is enlarged or the page is zoomed.

Important status changes and validation feedback are available to assistive
technology without unnecessarily interrupting the user.

Motion respects reduced-motion preferences and does not obstruct interaction or
understanding.

## Language

Interface language is clear, concise, and direct. Headings, labels, actions, and
messages use sentence case and a consistent tone.

The same concept uses the same term throughout the application. Different
operations use distinct language where their effects differ.

Action labels describe what will happen and include enough context to make their
meaning clear.

Terminology suits the intended audience. Technical terms are used where they
help users understand or complete the task.

Instructions and guidance explain relevant requirements or consequences without
repeating information already apparent from the interface.

Messages describe outcomes and problems in plain language, without blaming the
user.

Dates, times, numbers, file sizes, and units use consistent, understandable
formatting.
