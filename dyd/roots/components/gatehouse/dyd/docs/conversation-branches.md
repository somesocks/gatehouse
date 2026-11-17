# Conversational branches

`GET /api/v1/workspaces/{workspace}/sessions/{session}/events/branch` reads one
conversational branch: events sharing the same actual parent. Omitting `parent`
(or leaving it empty) selects the NULL-parent/root branch. Existing event-tree
and transcript endpoints are unchanged.

Parameters:

- `parent`: optional session-event ID in this workspace/session.
- `limit`: branch page size, default 20, range 1–100.
- `cursor`: opaque continuation returned as `next_cursor`. It is scoped to the
  workspace, session and parent; changing the page size is allowed.

Members are ordered oldest-first by `(created_at,id)`. Pagination is exclusive
and uses that pair, including equal-timestamp ID ties; it does not use OFFSET.
A cursor is a continuation, not a durable cross-page snapshot. Re-reading a
branch reflects current request/interaction state.

## Response

The response follows the generated `ConversationBranch` API schema:

- `parent`: scoped event reference, omitted for the root branch.
- `entries`: branch members. Each contains a public transcript `event`, up to
  two `previews`, and `has_more_replies`.
- `agent_requests`: state for each agent request visible as a branch member or
  preview. Each contains its `request` reference, `state`, interaction references
  and an optional terminal `outcome` event.
- `interactions`: input/approval `request` events and their optional `response`
  events, including requests displayed directly as members/previews.
- `next_cursor`: present only if another branch member was found.

All three collections are arrays, including when empty. References connect the
collections without inventing new parent relationships. In particular, an agent
outcome still names its owning agent request as its actual parent. Replies to an
outcome are read by selecting that outcome ID as `parent`.

Conversational members are `message.text`, `agent.request`, `agent.success`,
`agent.failure`, `input.request` and `approval.request`. Thinking and tool
execution events are not conversational members.

For each branch member other than an agent/input/approval request, selection
fetches at most three conversational direct-child candidates. Only the first
two are hydrated and expanded. The third sets `has_more_replies`; it is not
returned as a preview or expanded into outcomes/interactions. Agent requests
have their terminal outcome loaded separately rather than spending another
preview slot on it. Input/approval requests expose resolution state separately.
Previews themselves do not recursively acquire further previews in this read.

States are `working`, `waiting`, `cancelling`, `completed`, `failed` and
`cancelled`. A terminal outcome takes precedence over working/waiting/cancelling.
Absent an outcome, a cancellation request yields `cancelling`; otherwise any
unresolved owned interaction yields `waiting`, and the remaining state is
`working` (including queued/admitted work). Resolved historical interactions are
retained. Each selected agent request has independent state.

## Authorization and public projection

The caller must retain normal access to the chat on every request. Unauthorized
or missing chats and unavailable/cross-session parents return 404; missing
credentials return 401. Malformed selectors, limits or cursors return 400.

Events use the existing public transcript projection, batch file-metadata
hydration and historical agent attribution. Form schemas, input drafts,
submitted values, tool code and provider reasoning are not included. The
existing scoped input APIs remain the entry points for opening and responding
to forms; the branch read grants no additional permission. Approval submission
also continues to use the existing authorized approval endpoint.

## Database selection and cost

`SessionConversationBranchGet` selects branch membership, previews, outcomes,
cancellations and interaction resolution in one SQL statement/snapshot on
SQLite and PostgreSQL. HTTP authorization, attachment metadata and agent display
metadata use their existing separate reads. No file bodies are loaded.

Only selected branch/preview membership is bounded by the page/preview limits.
Cancellation discovery can scan a selected request's children. Interactive
lookup can walk all of its direct tool requests and return all owned historical
or pending interactions. This is not a bounded-frontier execution query, and a
long-running request can make the read substantially more expensive even when
its conversational output is small. Visible-kind filtering can also inspect
non-conversational siblings. No new ownership columns, indexes or lookup tables
are introduced by this API.
